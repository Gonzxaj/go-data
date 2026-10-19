// ---------- Contenedores ----------

const dockerGrid = document.getElementById("grid-docker");
const dockerCount = document.getElementById("docker-count");
let containerNodes = {};
let lastContainerKey = "";

function stateColor(state) {
  switch ((state || "").toLowerCase()) {
    case "running": return "var(--primary)";
    case "exited": case "dead": return "var(--crit)";
    case "restarting": case "paused": return "var(--warning)";
    default: return "var(--muted)";
  }
}

function buildContainerCard(name) {
  const el = document.createElement("article");
  el.className = "container-card";

  const head = document.createElement("div");
  head.className = "container-head";
  const dot = document.createElement("span");
  dot.className = "dot";
  const h3 = document.createElement("h3");
  h3.textContent = name;
  const restart = document.createElement("span");
  restart.className = "tag cc-restart";
  restart.style.display = "none"; // .tag sets display:inline-block, which as an author rule
  head.append(dot, h3, restart);  // beats the UA [hidden] rule regardless of specificity

  const status = document.createElement("p");
  status.className = "note mono";

  const tagsTop = document.createElement("div");
  tagsTop.className = "tags";
  tagsTop.style.marginTop = "9px";
  const image = document.createElement("span");
  image.className = "tag";
  const health = document.createElement("span");
  health.className = "tag";
  health.style.display = "none";
  tagsTop.append(image, health);

  const miniboxes = document.createElement("div");
  miniboxes.className = "miniboxes";
  function minibox(labelText) {
    const box = document.createElement("div");
    box.className = "minibox";
    const p1 = document.createElement("p");
    p1.className = "eyebrow";
    p1.textContent = labelText;
    const p2 = document.createElement("p");
    p2.className = "mono";
    box.append(p1, p2);
    return { box, valueEl: p2 };
  }
  const cpuBox = minibox("CPU");
  const memBox = minibox("Memoria");
  miniboxes.append(cpuBox.box, memBox.box);

  function metaRow(labelText) {
    const row = document.createElement("div");
    row.className = "resource-meta";
    row.style.marginTop = "8px";
    const a = document.createElement("span");
    a.textContent = labelText;
    const b = document.createElement("span");
    row.append(a, b);
    return { row, valueEl: b };
  }
  const ioRow = metaRow("E/S");
  const netRow = metaRow("Red");
  const sizeRow = metaRow("Disco");

  const ports = document.createElement("div");
  ports.className = "tags";

  el.append(head, status, tagsTop, miniboxes, ioRow.row, netRow.row, sizeRow.row, ports);

  return { el, dot, status, restart, image, health, cpu: cpuBox.valueEl, mem: memBox.valueEl, io: ioRow.valueEl, net: netRow.valueEl, size: sizeRow.valueEl, ports };
}

function rebuildContainerDom(containers) {
  containerNodes = {};
  dockerGrid.replaceChildren();
  if (!containers.length) {
    const note = document.createElement("p");
    note.className = "note";
    note.textContent = "No se detectaron contenedores en este host.";
    dockerGrid.appendChild(note);
    return;
  }
  for (const c of containers) {
    const node = buildContainerCard(c.name);
    dockerGrid.appendChild(node.el);
    containerNodes[c.name] = node;
  }
}

function portChip(c) {
  const span = document.createElement("span");
  span.className = "tag";
  span.textContent = c.label;
  if (!c.published) { span.style.color = "var(--muted)"; span.style.background = "rgba(141,163,180,.06)"; }
  return span;
}

function paintContainerCard(node, c) {
  const isRunning = c.state === "running";
  node.dot.style.background = stateColor(c.state);
  node.dot.style.animation = isRunning ? "" : "none";
  node.status.textContent = c.status || "";

  if (c.restart_count) {
    node.restart.style.display = "";
    node.restart.textContent = `↻ ${c.restart_count}`;
    node.restart.title = `Se reinició ${c.restart_count} veces desde que se creó`;
    if (c.restart_count >= 5) { node.restart.style.color = "var(--crit)"; node.restart.style.background = "rgba(226,102,92,.12)"; node.restart.style.borderColor = "rgba(226,102,92,.3)"; }
    else { node.restart.style.color = "var(--warning)"; node.restart.style.background = "rgba(226,185,81,.12)"; node.restart.style.borderColor = "rgba(226,185,81,.3)"; }
  } else {
    node.restart.style.display = "none";
  }

  node.image.textContent = c.image || "";

  if (c.health && c.health !== "none") {
    node.health.style.display = "";
    node.health.textContent = c.health;
    node.health.title = "Resultado del healthcheck definido en el contenedor";
    if (c.health === "healthy") { node.health.style.color = "var(--primary)"; node.health.style.background = "rgba(92,189,185,.12)"; }
    else if (c.health === "unhealthy") { node.health.style.color = "var(--crit)"; node.health.style.background = "rgba(226,102,92,.12)"; node.health.style.borderColor = "rgba(226,102,92,.3)"; }
    else { node.health.style.color = "var(--warning)"; node.health.style.background = "rgba(226,185,81,.12)"; node.health.style.borderColor = "rgba(226,185,81,.3)"; }
  } else {
    node.health.style.display = "none";
  }

  const dash = isRunning ? "N/D" : "detenido";

  if (isRunning && Number.isFinite(c.cpu_pct)) node.cpu.textContent = `${num(c.cpu_pct, 1)} %`;
  else node.cpu.textContent = dash;

  if (isRunning && c.mem_limit_bytes) node.mem.textContent = `${fmtBytes(c.mem_used_bytes)} · ${num(c.mem_pct, 0)}%`;
  else node.mem.textContent = dash;

  node.size.textContent = (c.size_rw_bytes || c.volume_usage_bytes)
    ? [c.size_rw_bytes && `capa ${fmtBytes(c.size_rw_bytes)}`, c.volume_usage_bytes && `vol ${fmtBytes(c.volume_usage_bytes)}`].filter(Boolean).join(" · ")
    : "N/D";

  node.io.textContent = isRunning
    ? `↓${fmtRate((c.block_read_bps || 0) / 1024, ["K", "M"])} ↑${fmtRate((c.block_write_bps || 0) / 1024, ["K", "M"])}`
    : dash;

  node.net.textContent = isRunning
    ? `↓${fmtRate((c.net_rx_bps || 0) / 1024, ["kb", "Mb"])} ↑${fmtRate((c.net_tx_bps || 0) / 1024, ["kb", "Mb"])}`
    : dash;

  node.ports.replaceChildren();
  const chips = c.port_chips || [];
  if (!chips.length) {
    const span = document.createElement("span");
    span.className = "tag";
    span.style.color = "var(--muted)";
    span.textContent = "sin puertos";
    node.ports.appendChild(span);
  } else {
    for (const chip of chips) node.ports.appendChild(portChip(chip));
  }
}

function renderSockets(stats) {
  const s = stats.sockets || {};
  const rows = [
    { label: "TCP en uso", value: s.tcp_in_use },
    { label: "TCP orphan", value: s.tcp_orphan },
    { label: "TCP time-wait", value: s.tcp_time_wait },
    { label: "UDP en uso", value: s.udp_in_use },
    { label: "Total sockets", value: s.total_used },
  ].filter((r) => Number.isFinite(r.value));
  setRows(document.getElementById("sockets-list"), rows.map((r) => ({ label: r.label, value: String(r.value) })), "sin datos de sockets");
}

function renderPublishedPorts(containers) {
  const rows = [];
  for (const c of containers) {
    for (const chip of (c.port_chips || []).filter((ch) => ch.published)) {
      rows.push({ sortKey: parseInt(chip.label, 10) || 0, label: chip.label, name: c.name });
    }
  }
  rows.sort((a, b) => a.sortKey - b.sortKey);
  setRows(document.getElementById("published-ports-list"), rows.map((r) => ({ label: r.label, value: `→ ${r.name}` })), "ningún contenedor publica puertos");
}

async function tickContainers() {
  let containers = [];
  try {
    const json = await fetchJSON("/api/containers");
    containers = (json.containers || []).slice().sort((a, b) => a.name.localeCompare(b.name));
  } catch {
    dockerCount.textContent = "no disponible";
    setRows(document.getElementById("published-ports-list"), [], "no disponible");
    return;
  }

  const key = containers.map((c) => c.name).join("|");
  if (key !== lastContainerKey) {
    rebuildContainerDom(containers);
    lastContainerKey = key;
  }
  dockerCount.textContent = containers.length ? `${containers.length} contenedor${containers.length === 1 ? "" : "es"}` : "0 contenedores";
  renderPublishedPorts(containers);

  for (const c of containers) {
    const node = containerNodes[c.name];
    if (node) paintContainerCard(node, c);
  }
}
