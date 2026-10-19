const HISTORY_POINTS = 60;
const REFRESH_MS = 2000;
const REDISCOVER_EVERY = 15; // re-lee host info / uptime sessions cada 15 ticks (~30s)

// ---------- helpers ----------

const clamp = (v) => Math.max(0, Math.min(100, v));
const num = (v, d = 1) => (Number.isFinite(v) ? v.toFixed(d) : "—");

function color(pct) {
  if (pct >= 90) return "var(--crit)";
  if (pct >= 70) return "var(--warning)";
  return "var(--primary)";
}
function colorCustom(v, warnAt, critAt) {
  if (v >= critAt) return "var(--crit)";
  if (v >= warnAt) return "var(--warning)";
  return "var(--primary)";
}

function fmtRate(value, unit) {
  const abs = Math.abs(value);
  if (abs >= 1024) return `${num(abs / 1024, 1)} ${unit[1]}`;
  return `${num(abs, 0)} ${unit[0]}`;
}
function fmtMem(mib) {
  if (!Number.isFinite(mib)) return "—";
  if (Math.abs(mib) >= 1024) return `${num(mib / 1024, 1)} GiB`;
  return `${num(mib, 0)} MiB`;
}
function fmtBytes(bytes) {
  if (!Number.isFinite(bytes)) return null;
  return fmtMem(bytes / 1024 / 1024);
}
function fmtKB(kb) {
  if (!Number.isFinite(kb)) return null;
  return fmtMem(kb / 1024);
}
function fmtUptime(seconds) {
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const parts = [];
  if (d) parts.push(`${d}d`);
  if (d || h) parts.push(`${h}h`);
  parts.push(`${m}m`);
  return parts.join(" ");
}
function fmtBootDate(bootTimeMs) {
  const boot = new Date(bootTimeMs);
  const date = boot.toLocaleDateString(undefined, { day: "2-digit", month: "2-digit", year: "numeric" });
  const time = boot.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
  return `activo desde ${date} ${time}`;
}
function fmtAgo(iso) {
  const sec = Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000);
  if (sec < 60) return "hace instantes";
  if (sec < 3600) return `hace ${Math.floor(sec / 60)} min`;
  if (sec < 86400) return `hace ${Math.floor(sec / 3600)} h`;
  return `hace ${Math.floor(sec / 86400)} d`;
}
function fmtEventTime(iso) {
  return new Date(iso).toLocaleString(undefined, { day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

async function fetchJSON(path) {
  const res = await fetch(path);
  if (res.status === 401 && authMode === "session") location.replace("/login.html");
  if (!res.ok) throw new Error(res.status);
  return res.json();
}
async function fetchHistory(metric, of) {
  const qs = new URLSearchParams({ metric, points: HISTORY_POINTS });
  if (of) qs.set("of", of);
  try {
    const json = await fetchJSON(`/api/history?${qs}`);
    const points = json.points || [];
    return { values: points.map((p) => p.value), times: points.map((p) => p.time) };
  } catch {
    return { values: [], times: [] };
  }
}
function sumSeries(a, b) {
  const n = Math.max(a.length, b.length);
  const out = [];
  for (let i = 0; i < n; i++) out.push((a[i] || 0) + (b[i] || 0));
  return out;
}

// ---------- info-list rows (DOM-built, never string-interpolated: values come
// straight from the API — some are attacker-influenced, e.g. SSH usernames
// or container names — so we never pass them through innerHTML) ----------

function setRows(ul, rows, emptyMsg) {
  ul.replaceChildren();
  if (!rows.length) {
    const li = document.createElement("li");
    const note = document.createElement("span");
    note.className = "note";
    note.style.margin = "0";
    note.textContent = emptyMsg;
    li.appendChild(note);
    ul.appendChild(li);
    return;
  }
  for (const r of rows) {
    const li = document.createElement("li");
    const a = document.createElement("span");
    a.textContent = r.label;
    const b = document.createElement("span");
    b.textContent = r.value;
    li.append(a, b);
    ul.appendChild(li);
  }
}
function setConnRows(container, rows, emptyMsg) {
  container.replaceChildren();
  if (!rows.length) {
    const note = document.createElement("p");
    note.className = "note";
    note.textContent = emptyMsg;
    container.appendChild(note);
    return;
  }
  for (const r of rows) {
    const row = document.createElement("div");
    row.className = "connection";
    const a = document.createElement("span");
    a.textContent = r.label;
    const b = document.createElement("span");
    b.textContent = r.value;
    row.append(a, b);
    container.appendChild(row);
  }
}
