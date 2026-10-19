// ---------- Seguridad ----------

const SSH_ACTION_LABEL = {
  accepted: "✔ acceso aceptado",
  failed: "✖ password incorrecta",
  invalid_user: "✖ usuario inválido",
  closed: "◦ conexión cerrada",
};

async function tickSSH() {
  const icon = document.getElementById("ssh-status-icon");
  const title = document.getElementById("ssh-status-title");
  const note = document.getElementById("ssh-status-note");
  try {
    const json = await fetchJSON("/api/security/ssh");
    if (!json.enabled) {
      icon.style.color = "var(--muted)";
      title.textContent = "SSH watch deshabilitado";
      note.textContent = "activá ssh_watch_enabled en config.json";
      setConnRows(document.getElementById("ssh-list"), [], "deshabilitado (activá ssh_watch_enabled en config.json)");
      return;
    }
    const events = json.events || [];
    if (!events.length) {
      icon.style.color = "";
      title.textContent = "Sin intentos sospechosos";
      note.textContent = "Últimas conexiones registradas";
    } else {
      icon.style.color = "var(--warning)";
      title.textContent = `${events.length} intento${events.length === 1 ? "" : "s"} registrado${events.length === 1 ? "" : "s"}`;
      note.textContent = "revisá el detalle a la derecha";
    }
    const rows = events.map((e) => ({ label: `${fmtEventTime(e.time)} · ${e.ip}${e.user ? " · " + e.user : ""}`, value: SSH_ACTION_LABEL[e.action] || e.action }));
    setConnRows(document.getElementById("ssh-list"), rows, "sin intentos registrados todavía");
  } catch {
    icon.style.color = "var(--muted)";
    title.textContent = "No disponible";
    note.textContent = "no se pudo consultar el estado de SSH";
    setConnRows(document.getElementById("ssh-list"), [], "no disponible");
  }
}

async function tickConnections() {
  try {
    const json = await fetchJSON("/api/security/connections");
    if (!json.enabled) {
      setConnRows(document.getElementById("conn-list"), [], "deshabilitado (activá conn_watch_enabled y conn_watch_ports en config.json)");
      return;
    }
    const rows = (json.events || []).map((e) => ({ label: `${fmtEventTime(e.time)} · puerto ${e.local_port}`, value: e.ip }));
    setConnRows(document.getElementById("conn-list"), rows, "sin conexiones registradas todavía");
  } catch {
    setConnRows(document.getElementById("conn-list"), [], "no disponible");
  }
}
