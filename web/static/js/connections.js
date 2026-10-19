// ---------- Conexiones (network monitor) ----------

function netCapabilityLabel(capability) {
  return capability === "conntrack" ? "" : "visibilidad limitada: solo TCP";
}

async function tickNetwork() {
  const capTag = document.getElementById("netmon-capability");

  try {
    const summary = await fetchJSON("/api/network/summary");
    if (!summary.enabled) {
      capTag.hidden = true;
      for (const id of ["nm-active", "nm-unique", "nm-tcp", "nm-udp", "nm-inbound", "nm-outbound"]) {
        document.getElementById(id).textContent = "—";
      }
    } else {
      const label = netCapabilityLabel(summary.capability);
      capTag.hidden = !label;
      capTag.textContent = label;
      document.getElementById("nm-active").textContent = String(summary.active_total);
      document.getElementById("nm-unique").textContent = String(summary.unique_ips);
      document.getElementById("nm-tcp").textContent = String(summary.active_tcp);
      document.getElementById("nm-udp").textContent = String(summary.active_udp);
      document.getElementById("nm-inbound").textContent = String(summary.inbound);
      document.getElementById("nm-outbound").textContent = String(summary.outbound);
      document.getElementById("nm-outbound-card").hidden = !summary.outbound_enabled;
    }
  } catch { /* keep last known values */ }

  // netmon has its own /api/network/history (separate ring buffer/cadence
  // from /api/history), so it needs its own small fetch rather than reusing
  // fetchHistory(), which is hard-wired to /api/history.
  try {
    const qs = new URLSearchParams({ metric: "active_total", points: HISTORY_POINTS });
    const json = await fetchJSON(`/api/network/history?${qs}`);
    const values = (json.points || []).map((p) => p.value);
    const times = (json.points || []).map((p) => p.time);
    netmonChart.render(values, times, "var(--primary)", (v) => `${num(v, 0)} activas`);
  } catch { /* leave the chart as-is */ }

  try {
    const conns = await fetchJSON("/api/network/connections");
    const tbody = document.getElementById("netmon-connections-body");
    tbody.replaceChildren();
    const rows = conns.enabled ? conns.connections : [];
    if (!rows.length) {
      const tr = document.createElement("tr");
      const td = document.createElement("td");
      td.colSpan = 5;
      td.className = "note";
      td.textContent = conns.enabled ? "sin conexiones activas" : "no disponible";
      tr.appendChild(td);
      tbody.appendChild(tr);
    } else {
      for (const c of rows) {
        const tr = document.createElement("tr");
        const cells = [c.remote_ip, c.proto.toUpperCase(), String(c.local_port), c.direction === "inbound" ? "entrante" : "saliente", c.state];
        for (const text of cells) {
          const td = document.createElement("td");
          td.textContent = text;
          tr.appendChild(td);
        }
        tbody.appendChild(tr);
      }
    }
  } catch {
    const tbody = document.getElementById("netmon-connections-body");
    tbody.replaceChildren();
    const tr = document.createElement("tr");
    const td = document.createElement("td");
    td.colSpan = 5;
    td.className = "note";
    td.textContent = "no disponible";
    tr.appendChild(td);
    tbody.appendChild(tr);
  }

  try {
    const top = await fetchJSON("/api/network/top?by=ip&n=10");
    setRows(document.getElementById("netmon-top-ips"),
      (top.enabled ? top.items : []).map((i) => ({ label: i.key, value: `${i.count} conexión${i.count === 1 ? "" : "es"}` })),
      "sin datos");
  } catch {
    setRows(document.getElementById("netmon-top-ips"), [], "no disponible");
  }

  try {
    const top = await fetchJSON("/api/network/top?by=port&n=10");
    setRows(document.getElementById("netmon-top-ports"),
      (top.enabled ? top.items : []).map((i) => ({ label: i.key, value: `${i.count} conexión${i.count === 1 ? "" : "es"}` })),
      "sin datos");
  } catch {
    setRows(document.getElementById("netmon-top-ports"), [], "no disponible");
  }

  try {
    const json = await fetchJSON("/api/network/events?n=50");
    const rows = (json.enabled ? json.events : []).map((e) => ({
      label: `${fmtEventTime(e.time)} · ${e.remote_ip} · ${e.proto.toUpperCase()} · :${e.local_port}`,
      value: e.direction === "inbound" ? "entrante" : "saliente",
    }));
    setConnRows(document.getElementById("netmon-events-list"), rows, "sin conexiones registradas todavía");
  } catch {
    setConnRows(document.getElementById("netmon-events-list"), [], "no disponible");
  }

  try {
    const json = await fetchJSON("/api/network/ip-history?n=100");
    const rows = (json.enabled ? json.entries : []).map((e) => ({
      label: e.ip,
      value: `${e.connections} conexión${e.connections === 1 ? "" : "es"} · desde ${fmtEventTime(e.first_seen)} · última ${fmtEventTime(e.last_seen)}`,
    }));
    setRows(document.getElementById("netmon-ip-history"), rows, "sin historial todavía");
  } catch {
    setRows(document.getElementById("netmon-ip-history"), [], "no disponible");
  }
}
