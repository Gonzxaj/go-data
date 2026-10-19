// ---------- General: uptime, recursos, alertas, hardware ----------

async function tickSysInfo() {
  try {
    const info = await fetchJSON("/api/host");
    cachedHost = info;
    systemCores = info.cores_total || null;
    document.getElementById("hostname").textContent = info.hostname || "servidor";
    document.getElementById("uptime-tag").textContent = info.kernel_version || "—";

    setRows(document.getElementById("hw-list"), [
      { label: "Hostname", value: info.hostname || "—" },
      { label: "Sistema", value: [info.os_name, info.os_version].filter(Boolean).join(" ") || "—" },
      { label: "Kernel", value: info.kernel_version || "—" },
      { label: "Arquitectura", value: info.architecture || "—" },
      { label: "Procesador", value: [info.cpu_vendor, info.cpu_model].filter(Boolean).join(" ") || "—" },
      { label: "Núcleos", value: info.cores_total != null ? String(info.cores_total) : "—" },
      { label: "Frecuencia CPU", value: info.cpu_mhz ? `${num(info.cpu_mhz / 1000, 2)} GHz${info.cpu_mhz_is_max ? " (máx.)" : ""}` : "—" },
      { label: "RAM total", value: fmtKB(info.ram_total_kb) || "—" },
      { label: "Frecuencia RAM", value: info.ram_frequency_mhz ? `${num(info.ram_frequency_mhz, 0)} MHz` : "N/D" },
      { label: "Virtualización", value: info.virtualization && info.virtualization !== "none" ? info.virtualization : "—" },
    ], "no disponible");

    renderDisks(info.disks || []);

    setRows(document.getElementById("usb-list"),
      (info.usb_devices || []).map((d) => ({ label: d.product || `${d.vendor_id}:${d.product_id}`, value: d.manufacturer || `${d.vendor_id}:${d.product_id}` })),
      "sin dispositivos USB detectados");
  } catch {
    setRows(document.getElementById("hw-list"), [], "no disponible");
    renderDisks([]);
    setRows(document.getElementById("usb-list"), [], "sin dispositivos USB detectados");
  }
}

// Renders physical disks as cards, each listing only its relevant (mounted,
// data-bearing) partitions — EFI/boot/swap partitions were already filtered
// out server-side. Built with DOM APIs like setRows, for the same reason:
// device/model/mount strings are backend-reported but never interpolated
// into markup.
function renderDisks(disks) {
  const root = document.getElementById("disks-list");
  root.replaceChildren();
  if (!disks.length) {
    root.className = "";
    const li = document.createElement("p");
    li.className = "note";
    li.style.margin = "16px 0 0";
    li.textContent = "sin discos físicos detectados";
    root.appendChild(li);
    return;
  }
  root.className = "disk-group";
  root.style.marginTop = "16px";
  for (const disk of disks) {
    root.appendChild(renderDiskCard(disk));
  }
}

function renderDiskCard(disk) {
  const card = document.createElement("div");
  card.className = "disk-card";

  const head = document.createElement("div");
  head.className = "disk-head";
  const device = document.createElement("span");
  device.className = "device";
  device.textContent = [disk.device, [disk.model, disk.kind].filter(Boolean).join(" · ")].filter(Boolean).join(" · ");
  const cap = document.createElement("span");
  cap.className = "cap mono";
  cap.textContent = fmtBytes(disk.size_bytes) || "—";
  head.append(device, cap);
  card.appendChild(head);

  const partitions = disk.partitions || [];
  const list = document.createElement("div");
  list.className = "disk-partitions";
  if (!partitions.length) {
    const note = document.createElement("p");
    note.className = "note";
    note.style.margin = "10px 0 0";
    note.textContent = "sin particiones con datos detectadas";
    list.appendChild(note);
  } else {
    for (const p of partitions) list.appendChild(renderPartitionRow(p));
  }
  card.appendChild(list);
  return card;
}

function renderPartitionRow(p) {
  const row = document.createElement("div");
  row.className = "partition";

  const head = document.createElement("div");
  head.className = "partition-head";
  const mount = document.createElement("span");
  mount.className = "partition-mount mono";
  mount.textContent = p.mount || p.device;
  const meta = document.createElement("span");
  meta.className = "partition-meta mono";
  meta.textContent = [p.device, p.fs_type].filter(Boolean).join(" · ");
  head.append(mount, meta);

  const usage = document.createElement("div");
  usage.className = "partition-usage mono";
  const used = document.createElement("span");
  used.textContent = `${fmtBytes(p.used_bytes) || "—"} / ${fmtBytes(p.total_bytes) || "—"} · ${num(p.used_pct, 1)}% usado`;
  const avail = document.createElement("span");
  avail.textContent = `${fmtBytes(p.avail_bytes) || "—"} disponibles`;
  usage.append(used, avail);

  const bar = document.createElement("div");
  bar.className = "barline";
  const fill = document.createElement("div");
  fill.className = "fill";
  fill.style.width = `${clamp(p.used_pct)}%`;
  fill.style.background = color(p.used_pct);
  bar.appendChild(fill);

  row.append(head, usage, bar);
  return row;
}

// Downtime = gaps between one session's last sighting and the next boot.
// Days before the first recorded session have no data.
const UPTIME_DAYS = 90;
function renderUptimeDays(sessions) {
  const now = Date.now();
  const chrono = sessions.map((s) => ({ start: new Date(s.start).getTime(), end: s.end ? new Date(s.end).getTime() : now })).reverse();
  const gaps = [];
  for (let i = 1; i < chrono.length; i++) {
    if (chrono[i].start > chrono[i - 1].end) gaps.push([chrono[i - 1].end, chrono[i].start]);
  }
  const firstSeen = chrono.length ? chrono[0].start : now;
  const overlap = (a0, a1, b0, b1) => Math.max(0, Math.min(a1, b1) - Math.max(a0, b0));

  const box = document.getElementById("uptime-days");
  box.replaceChildren();
  let observedTotal = 0, downTotal = 0;
  const today = new Date(); today.setHours(0, 0, 0, 0);
  for (let i = UPTIME_DAYS - 1; i >= 0; i--) {
    const d0 = new Date(today); d0.setDate(today.getDate() - i);
    const d1 = new Date(d0); d1.setDate(d0.getDate() + 1);
    const observed = overlap(d0.getTime(), d1.getTime(), firstSeen, now);
    const down = gaps.reduce((a, [g0, g1]) => a + overlap(d0.getTime(), d1.getTime(), g0, g1), 0);
    observedTotal += observed; downTotal += down;

    const bar = document.createElement("div");
    bar.className = "uptime-day";
    const date = d0.toLocaleDateString(undefined, { day: "2-digit", month: "2-digit", year: "numeric" });
    if (!observed) { bar.classList.add("nodata"); bar.title = `${date} · sin datos`; }
    else {
      if (down > 0) bar.classList.add(down / observed > 0.01 ? "major" : "minor");
      const pct = (100 * (1 - down / observed)).toFixed(2);
      bar.title = down > 0 ? `${date} · ${pct}% · caído ${fmtUptime(down / 1000)}` : `${date} · ${pct}% · sin caídas`;
    }
    box.appendChild(bar);
  }
  document.getElementById("uptime-days-pct").textContent = observedTotal
    ? `${(100 * (1 - downTotal / observedTotal)).toFixed(2)} % uptime`
    : "sin datos";
}

async function tickUptimeSessions() {
  let sessions = [];
  try {
    const json = await fetchJSON("/api/uptime/sessions");
    sessions = json.sessions || [];
  } catch { /* keep last known */ }

  renderUptimeDays(sessions);
  const past = sessions.filter((s) => !s.ongoing);
  document.getElementById("stat-restarts").textContent = String(past.length);
  document.getElementById("stat-avg").textContent = past.length
    ? fmtUptime(past.reduce((a, s) => a + s.duration_seconds, 0) / past.length)
    : "—";
  document.getElementById("stat-record").textContent = past.length
    ? fmtUptime(Math.max(...past.map((s) => s.duration_seconds)))
    : "—";

  const tbody = document.getElementById("sessions-body");
  tbody.replaceChildren();
  const shown = sessions.slice(0, 8);
  if (!shown.length) {
    const tr = document.createElement("tr");
    const td = document.createElement("td");
    td.colSpan = 3;
    td.className = "note";
    td.textContent = "sin historial de reinicios todavía";
    tr.appendChild(td);
    tbody.appendChild(tr);
    return;
  }
  const maxDur = Math.max(...shown.map((s) => s.duration_seconds), 1);
  for (const s of shown) {
    const tr = document.createElement("tr");

    const tdDate = document.createElement("td");
    tdDate.className = "mono";
    tdDate.style.color = "var(--muted)";
    tdDate.textContent = s.ongoing ? "actual" : new Date(s.start).toLocaleDateString(undefined, { day: "2-digit", month: "2-digit" });

    const tdDur = document.createElement("td");
    const wrap = document.createElement("div");
    wrap.style.display = "flex";
    wrap.style.flexDirection = "column";
    wrap.style.gap = "4px";
    const bar = document.createElement("div");
    bar.className = "barline";
    bar.style.margin = "0";
    bar.title = fmtUptime(s.duration_seconds);
    const fill = document.createElement("div");
    fill.className = "fill";
    const pct = clamp((s.duration_seconds / maxDur) * 100);
    fill.style.width = `${pct}%`;
    fill.style.background = s.ongoing ? "var(--primary)" : s.duration_seconds < 600 ? "var(--crit)" : s.duration_seconds < 7200 ? "var(--warning)" : "var(--primary)";
    bar.appendChild(fill);
    const label = document.createElement("span");
    label.className = "mono";
    label.style.fontSize = "10.5px";
    label.style.color = "var(--muted)";
    label.textContent = fmtUptime(s.duration_seconds);
    wrap.append(bar, label);
    tdDur.appendChild(wrap);

    const tdState = document.createElement("td");
    const tag = document.createElement("span");
    tag.className = "tag";
    if (s.ongoing) { tag.textContent = "actual"; }
    else if (s.duration_seconds < 600) { tag.textContent = "breve"; tag.style.color = "var(--crit)"; tag.style.background = "rgba(226,102,92,.12)"; tag.style.borderColor = "rgba(226,102,92,.3)"; }
    else if (s.duration_seconds < 7200) { tag.textContent = "corta"; tag.style.color = "var(--warning)"; tag.style.background = "rgba(226,185,81,.12)"; tag.style.borderColor = "rgba(226,185,81,.3)"; }
    else { tag.textContent = "estable"; }
    tdState.appendChild(tag);

    tr.append(tdDate, tdDur, tdState);
    tbody.appendChild(tr);
  }
}

function paintGeneral(stats) {
  if (cachedHost?.boot_time) {
    const seconds = (Date.now() - new Date(cachedHost.boot_time).getTime()) / 1000;
    document.getElementById("uptime-value").textContent = fmtUptime(seconds);
    document.getElementById("uptime-sub").textContent = fmtBootDate(new Date(cachedHost.boot_time).getTime());
  }

  const cpuPct = stats.cpu?.usage_pct;
  document.getElementById("gen-cpu-value").textContent = `${num(cpuPct, 1)}%`;
  document.getElementById("gen-cpu-fill").style.width = `${clamp(cpuPct || 0)}%`;
  document.getElementById("gen-cpu-fill").style.background = color(cpuPct || 0);
  document.getElementById("gen-cpu-detail").textContent = cachedHost
    ? `${cachedHost.cores_total ?? "—"} núcleos${cachedHost.cpu_mhz ? " · " + num(cachedHost.cpu_mhz / 1000, 2) + " GHz" : ""}`
    : "—";
  const t = primaryTemp(stats);
  document.getElementById("gen-cpu-temp").textContent = t ? `${num(t.celsius, 1)} °C` : "N/D";

  const ramPct = stats.ram?.used_pct;
  const ramValueEl = document.getElementById("gen-ram-value");
  if (cachedHost?.ram_total_kb) {
    ramValueEl.textContent = `${fmtKB(stats.ram?.used_kb) || "—"} / ${fmtKB(cachedHost.ram_total_kb)}`;
  } else {
    ramValueEl.textContent = fmtKB(stats.ram?.used_kb) || "—";
  }
  ramValueEl.style.color = color(ramPct || 0);
  document.getElementById("gen-ram-fill").style.width = `${clamp(ramPct || 0)}%`;
  document.getElementById("gen-ram-fill").style.background = color(ramPct || 0);
  document.getElementById("gen-ram-detail").textContent = `${num(ramPct, 1)}% en uso`;
  document.getElementById("gen-ram-avail").textContent = `${fmtKB(stats.ram?.available_kb) || "—"} libre`;
}

function renderAlerts(alarms) {
  const countEl = document.getElementById("alerts-count");
  const listEl = document.getElementById("alerts-list");
  const bell = document.getElementById("alert-bell");
  const badge = document.getElementById("alert-badge");

  countEl.textContent = String(alarms.length);
  badge.hidden = alarms.length === 0;
  bell.title = alarms.length ? `${alarms.length} alerta${alarms.length === 1 ? "" : "s"} activa${alarms.length === 1 ? "" : "s"}` : "sin alertas activas";

  listEl.replaceChildren();
  if (!alarms.length) {
    const note = document.createElement("p");
    note.className = "note";
    note.textContent = "sin alertas activas";
    listEl.appendChild(note);
    return;
  }
  for (const a of alarms) {
    const row = document.createElement("div");
    row.className = "alert";
    const dot = document.createElement("span");
    dot.className = "dot";
    dot.style.background = a.severity === "crit" ? "var(--crit)" : "var(--warning)";
    const label = document.createElement("span");
    label.className = "label";
    const isPct = a.metric && a.metric.endsWith("_pct");
    label.textContent = `${a.message}${isPct && Number.isFinite(a.value) ? ` (${num(a.value, 0)}%)` : ""}`;
    const time = document.createElement("span");
    time.className = "time";
    time.textContent = a.since ? fmtAgo(a.since) : "";
    row.append(dot, label, time);
    listEl.appendChild(row);
  }
}

// ---------- CPU: top procesos ----------

function renderAppsTop(stats) {
  const rows = (stats.processes || []).map((p) => ({ label: `${p.name} (${p.pid})`, value: `${num(p.cpu_pct, 1)} %` }));
  setRows(document.getElementById("cpu-procs"), rows, "sin datos de procesos");
}
