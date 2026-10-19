// ---------- métricas por sección ----------

let systemCores = null;
let cachedHost = null;

function diskOf(stats, mount = "/") {
  return (stats.disks || []).find((d) => d.mount === mount) || null;
}
function primaryTemp(stats) {
  const temps = stats.temp || [];
  return temps.find((t) => t.is_primary_cpu) || temps[0] || null;
}
function sumNet(stats, field) {
  return (stats.net || []).reduce((a, n) => a + (n[field] || 0), 0);
}
function sumDiskIO(stats, field) {
  return (stats.disk_io || []).reduce((a, d) => a + (d[field] || 0), 0);
}
function maxDiskIO(stats, field) {
  return (stats.disk_io || []).reduce((a, d) => Math.max(a, d[field] || 0), 0);
}

const METRICS = {
  cpu: [
    { key: "cpu-use", label: "Uso", history: "cpu.usage_pct", fmt: (v) => `${num(v, 1)} %`,
      compute: (s) => ({ value: `${num(s.cpu?.usage_pct, 1)} %`, note: `iowait ${num(s.cpu?.iowait_pct, 1)}% · sys ${num(s.cpu?.system_pct, 1)}%`, pct: s.cpu?.usage_pct }) },
    { key: "cpu-user", label: "Usuario", history: "cpu.user_pct", fmt: (v) => `${num(v, 1)} %`,
      compute: (s) => ({ value: `${num(s.cpu?.user_pct, 1)} %`, note: "procesos en espacio de usuario", pct: s.cpu?.user_pct }) },
    { key: "cpu-system", label: "Sistema", history: "cpu.system_pct", fmt: (v) => `${num(v, 1)} %`,
      compute: (s) => ({ value: `${num(s.cpu?.system_pct, 1)} %`, note: "llamadas al kernel", pct: s.cpu?.system_pct }) },
    { key: "cpu-iowait", label: "Iowait", colorFn: (v) => colorCustom(v, 10, 20), history: "cpu.iowait_pct", fmt: (v) => `${num(v, 1)} %`,
      compute: (s) => { const p = s.cpu?.iowait_pct ?? 0; return { value: `${num(p, 1)} %`, note: p >= 20 ? "cuello de botella de disco" : p >= 10 ? "esperando I/O, vigilar" : "sin espera relevante", pct: p }; } },
    { key: "cpu-steal", label: "Steal", colorFn: (v) => colorCustom(v, 5, 15), history: "cpu.steal_pct", fmt: (v) => `${num(v, 1)} %`,
      compute: (s) => { const p = s.cpu?.steal_pct ?? 0; return { value: `${num(p, 1)} %`, note: p > 0 ? "CPU robada por el hypervisor (VPS)" : "sin contención de hypervisor", pct: p }; } },
    { key: "cpu-load", label: "Promedio (load)", history: "load.load1", fmt: (v) => num(v, 2),
      compute: (s) => { const l = s.load || {}; return { value: num(l.load1, 2), note: systemCores ? `${num(l.normalized_pct, 0)}% de ${systemCores} núcleos · 5m ${num(l.load5, 2)} · 15m ${num(l.load15, 2)}` : `5m ${num(l.load5, 2)} · 15m ${num(l.load15, 2)}`, pct: systemCores ? clamp(l.normalized_pct) : null }; } },
    { key: "cpu-temp", label: "Temperatura", history: "temp.celsius", fmt: (v) => `${num(v, 1)} °C`,
      compute: (s) => { const t = primaryTemp(s); if (!t) return { value: "N/D", note: "sin sensor disponible" }; return { value: `${num(t.celsius, 1)} °C`, note: t.celsius >= 85 ? "alta" : t.celsius >= 70 ? "elevada" : "normal", pct: clamp(((t.celsius - 30) / (90 - 30)) * 100) }; } },
  ],
  ram: [
    { key: "ram-used", label: "En uso", history: "ram.used_pct", fmt: (v) => `${num(v, 1)} %`,
      compute: (s) => ({ value: `${num(s.ram?.used_pct, 1)} %`, note: fmtKB(s.ram?.used_kb) || "—", pct: s.ram?.used_pct }) },
    { key: "ram-avail", label: "Disponible", history: "ram.available_pct", fmt: (v) => `${num(v, 1)} %`,
      compute: (s) => ({ value: fmtKB(s.ram?.available_kb) || "—", note: "libre + caché + buffers", pct: s.ram?.available_pct }) },
    { key: "ram-free", label: "Free", history: "ram.free_pct", fmt: (v) => `${num(v, 1)} %`,
      compute: (s) => ({ value: fmtKB(s.ram?.free_kb) || "—", note: "sin caché ni buffers", pct: s.ram?.free_pct }) },
    { key: "ram-swap", label: "Swap", history: "swap.used_pct", fmt: (v) => `${num(v, 1)} %`,
      compute: (s) => { if (!s.swap?.present) return { value: "N/D", note: "sin swap configurado" }; return { value: `${num(s.swap.used_pct, 1)} %`, note: fmtKB(s.swap.used_kb) || "—", pct: s.swap.used_pct }; } },
    { key: "ram-cache", label: "Caché", history: "ram.cache_pct", fmt: (v) => `${num(v, 1)} %`,
      compute: (s) => ({ value: fmtKB(s.ram?.cached_kb) || "—", note: "reclamable, no es uso real", pct: s.ram?.cache_pct }) },
  ],
  disk: [
    { key: "disk-used", label: "En uso", history: "disk.used_pct", fmt: (v) => `${num(v, 1)} %`,
      compute: (s) => { const d = diskOf(s); if (!d) return { value: "—", note: "sin datos de /" }; return { value: `${num(d.used_pct, 1)} %`, note: `${num(d.used_bytes / 1024 / 1024 / 1024, 1)} GiB`, pct: d.used_pct }; } },
    { key: "disk-avail", label: "Disponible", history: "disk.avail_bytes", fmt: (v) => `${num(v / 1024 / 1024 / 1024, 1)} GiB`,
      compute: (s) => { const d = diskOf(s); if (!d) return { value: "—", note: "sin datos de /" }; return { value: `${num(d.avail_bytes / 1024 / 1024 / 1024, 1)} GiB`, note: "espacio libre en /", pct: clamp(100 - d.used_pct) }; } },
    { key: "disk-read", label: "Lectura", history: "disk_io.read_bps", fmt: (v) => fmtRate(v / 1024, ["KiB/s", "MiB/s"]),
      compute: (s) => ({ value: fmtRate(sumDiskIO(s, "read_bps") / 1024, ["KiB/s", "MiB/s"]), note: "disco, global" }) },
    { key: "disk-write", label: "Escritura", history: "disk_io.write_bps", fmt: (v) => fmtRate(v / 1024, ["KiB/s", "MiB/s"]),
      compute: (s) => ({ value: fmtRate(sumDiskIO(s, "write_bps") / 1024, ["KiB/s", "MiB/s"]), note: "disco, global" }) },
    { key: "disk-iops", label: "IOPS", history: "disk_io.read_iops", fmt: (v) => `${num(v, 0)} ops/s`,
      compute: (s) => { const iops = sumDiskIO(s, "read_iops") + sumDiskIO(s, "write_iops"); const devices = (s.disk_io || []).length; return { value: `${num(iops, 0)} ops/s`, note: devices ? `${devices} disco${devices === 1 ? "" : "s"}` : "sin dispositivos" }; } },
    { key: "disk-latency", label: "Latencia I/O", colorFn: (v) => colorCustom(v, 20, 50), history: "disk_io.latency_ms", fmt: (v) => `${num(v, 1)} ms`,
      compute: (s) => { if (!(s.disk_io || []).length) return { value: "N/D", note: "sin dispositivos de disco" }; const v = maxDiskIO(s, "latency_ms"); return { value: `${num(v, 1)} ms`, note: v >= 50 ? "latencia alta, posible cuello de botella" : v >= 20 ? "elevada, vigilar" : "normal", pct: clamp(v) }; } },
  ],
  net: [
    { key: "net-in", label: "Entrante", history: "net.rx_bps", fmt: (v) => fmtRate(v / 1024, ["kb/s", "Mb/s"]),
      compute: (s) => ({ value: fmtRate(sumNet(s, "rx_bps") / 1024, ["kb/s", "Mb/s"]), note: "↓ descarga" }) },
    { key: "net-out", label: "Saliente", history: "net.tx_bps", fmt: (v) => fmtRate(v / 1024, ["kb/s", "Mb/s"]),
      compute: (s) => ({ value: fmtRate(sumNet(s, "tx_bps") / 1024, ["kb/s", "Mb/s"]), note: "↑ subida" }) },
    { key: "net-total", label: "Consumo total", history: "net.rx_bps", extraHistory: "net.tx_bps", fmt: (v) => fmtRate(v / 1024, ["kb/s", "Mb/s"]),
      compute: (s) => { const rx = sumNet(s, "rx_bps"), tx = sumNet(s, "tx_bps"); return { value: fmtRate((rx + tx) / 1024, ["kb/s", "Mb/s"]), note: `↓ ${fmtRate(rx / 1024, ["kb/s", "Mb/s"])} · ↑ ${fmtRate(tx / 1024, ["kb/s", "Mb/s"])}` }; } },
    { key: "net-packets", label: "Paquetes", history: "net.packets_ps", fmt: (v) => `${num(v, 0)} pps`,
      compute: (s) => { const ifaces = (s.net || []).length; return { value: `${num(sumNet(s, "packets_ps"), 0)} pps`, note: ifaces ? `${ifaces} interfaz${ifaces === 1 ? "" : "es"}` : "sin interfaces" }; } },
    { key: "net-errors", label: "Errores / drops", colorFn: (v) => colorCustom(v, 1, 5), history: "net.errors_ps", extraHistory: "net.drops_ps", fmt: (v) => `${num(v, 2)} /s`,
      compute: (s) => { const v = sumNet(s, "errors_ps") + sumNet(s, "drops_ps"); return { value: `${num(v, 2)} /s`, note: v > 0 ? "hay errores o paquetes descartados" : "sin errores ni drops", pct: clamp(v) }; } },
  ],
};
const PRIMARY_HISTORY = {
  cpu: { history: "cpu.usage_pct", fmt: (v) => `${num(v, 1)} %` },
  ram: { history: "ram.used_pct", fmt: (v) => `${num(v, 1)} %` },
  disk: { history: "disk.used_pct", fmt: (v) => `${num(v, 1)} %` },
  net: { history: "net.rx_bps", extraHistory: "net.tx_bps", fmt: (v) => fmtRate(v / 1024, ["kb/s", "Mb/s"]) },
};

const cards = {};
for (const [section, list] of Object.entries(METRICS)) {
  const grid = document.getElementById(`metrics-${section}`);
  for (const metric of list) {
    const card = document.createElement("div");
    card.className = "metric";
    const eyebrow = document.createElement("p");
    eyebrow.className = "eyebrow";
    eyebrow.textContent = metric.label;
    const value = document.createElement("p");
    value.className = "metric-value";
    value.textContent = "—";
    const note = document.createElement("p");
    note.className = "note";
    const barline = document.createElement("div");
    barline.className = "barline";
    barline.hidden = true;
    const fill = document.createElement("div");
    fill.className = "fill";
    barline.appendChild(fill);
    const spark = createSpark(300, 80, 4, false);
    card.append(eyebrow, value, note, barline, spark.wrap);
    grid.appendChild(card);
    cards[metric.key] = { value, note, barline, fill, spark };
  }
}

const sectionCharts = {};
for (const section of Object.keys(PRIMARY_HISTORY)) {
  const mount = document.getElementById(`chart-${section}`);
  const chart = createSpark(800, 130, 10, true);
  mount.appendChild(chart.wrap);
  sectionCharts[section] = chart;
}

const netmonChart = createSpark(800, 130, 10, true);
document.getElementById("chart-netmon").appendChild(netmonChart.wrap);

function paintMetric(entry, metric, data) {
  entry.value.textContent = data.value;
  entry.value.style.color = data.pct === null || data.pct === undefined ? "" : (metric.colorFn || color)(data.pct);
  entry.note.textContent = data.note || "";
  const hasPct = data.pct !== null && data.pct !== undefined;
  entry.barline.hidden = !hasPct;
  if (hasPct) {
    entry.fill.style.width = `${clamp(data.pct)}%`;
    entry.fill.style.background = (metric.colorFn || color)(data.pct);
  }
  const stroke = hasPct ? (metric.colorFn || color)(data.pct) : "var(--primary)";
  entry.spark.render(data.spark, data.times, stroke, metric.fmt);
}
