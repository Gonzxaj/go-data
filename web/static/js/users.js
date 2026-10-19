// ---------- sesión y usuarios (solo AUTH_MODE=session) ----------
// Igual que el resto del panel: los datos (usernames) se insertan con
// textContent, nunca con innerHTML.

let authMode = null;
let currentUser = null;

async function sendJSON(method, path, body) {
  const res = await fetch(path, {
    method,
    headers: body ? { "Content-Type": "application/json" } : {},
    body: body ? JSON.stringify(body) : undefined,
  });
  if (res.status === 401 && authMode === "session" && path !== "/api/auth/password") location.replace("/login.html");
  if (res.ok) return res.status === 204 ? null : res.json();
  const json = await res.json().catch(() => ({}));
  throw new Error(json.error || `error ${res.status}`);
}

async function initAuth() {
  try {
    const me = await fetchJSON("/api/auth/me");
    authMode = me.mode;
    currentUser = me.user || null;
  } catch {
    return;
  }
  if (!currentUser) return;
  document.getElementById("user-chip").hidden = false;
  document.getElementById("user-avatar").textContent = currentUser.username.slice(0, 2).toUpperCase();
  document.getElementById("user-name").textContent = currentUser.username;
  document.getElementById("user-role").textContent = authMode === "session" ? currentUser.role : "basic auth";
  if (authMode !== "session") return;
  document.getElementById("btn-password").hidden = false;
  document.getElementById("btn-logout").hidden = false;
  document.getElementById("nav-users").hidden = currentUser.role !== "admin";
}

document.getElementById("btn-logout").addEventListener("click", async () => {
  try { await sendJSON("POST", "/api/auth/logout"); } catch {}
  location.replace("/login.html");
});

function setUsersMsg(text, kind = "") {
  const el = document.getElementById("users-msg");
  el.textContent = text;
  el.className = `form-msg ${kind}`;
}

function button(label, onClick, { danger = false, disabled = false } = {}) {
  const b = document.createElement("button");
  b.type = "button";
  b.className = danger ? "btn danger" : "btn";
  b.textContent = label;
  b.disabled = disabled;
  b.addEventListener("click", onClick);
  return b;
}

async function updateUser(u, patch, okMsg) {
  try {
    await sendJSON("PUT", `/api/users/${u.id}`, patch);
    setUsersMsg(okMsg, "ok");
  } catch (e) {
    setUsersMsg(e.message, "err");
  }
  loadUsers();
}

function renderUsers(list) {
  const body = document.getElementById("users-body");
  body.replaceChildren();
  for (const u of list) {
    const self = currentUser && u.id === currentUser.id;
    const tr = document.createElement("tr");

    const tdName = document.createElement("td");
    tdName.textContent = u.username;
    if (self) {
      const tag = document.createElement("span");
      tag.className = "tag";
      tag.style.marginLeft = "8px";
      tag.textContent = "vos";
      tdName.appendChild(tag);
    }

    // El rol se elige en el select y se aplica recién con "Guardar".
    const tdRole = document.createElement("td");
    const roleEdit = document.createElement("span");
    roleEdit.className = "role-edit";
    const sel = document.createElement("select");
    sel.className = "field";
    sel.setAttribute("aria-label", `Rol de ${u.username}`);
    for (const r of ["viewer", "admin"]) sel.add(new Option(r, r, false, r === u.role));
    sel.disabled = self;
    const save = button("Guardar", () => updateUser(u, { role: sel.value }, `${u.username} ahora es ${sel.value}`), { disabled: true });
    save.classList.add("primary");
    save.hidden = self;
    sel.addEventListener("change", () => { save.disabled = sel.value === u.role; });
    roleEdit.append(sel, save);
    tdRole.appendChild(roleEdit);

    const tdState = document.createElement("td");
    const state = document.createElement("span");
    state.className = `state ${u.active ? "on" : "off"}`;
    state.textContent = u.active ? "● activo" : "○ inactivo";
    tdState.appendChild(state);

    const tdCreated = document.createElement("td");
    tdCreated.className = "time";
    tdCreated.textContent = new Date(u.created_at).toLocaleDateString(undefined, { day: "2-digit", month: "2-digit", year: "numeric" });

    const tdActions = document.createElement("td");
    const actions = document.createElement("span");
    actions.className = "users-actions";
    actions.append(
      button(u.active ? "Desactivar" : "Activar", () => updateUser(u, { active: !u.active }, `${u.username} ${u.active ? "desactivado" : "activado"}`), { disabled: self }),
      button("Cambiar contraseña", () => openPasswordDialog(u)),
      button("Eliminar", () => openDeleteDialog(u), { danger: true, disabled: self }),
    );
    tdActions.appendChild(actions);

    tr.append(tdName, tdRole, tdState, tdCreated, tdActions);
    body.appendChild(tr);
  }
}

async function loadUsers() {
  if (authMode !== "session" || currentUser?.role !== "admin") return;
  try {
    renderUsers(await fetchJSON("/api/users"));
  } catch {
    setUsersMsg("no se pudo cargar la lista de usuarios", "err");
  }
}

// Todos los modales cierran con su botón "Cancelar" (data-close) o con Esc.
document.querySelectorAll("dialog [data-close]").forEach((b) => b.addEventListener("click", () => b.closest("dialog").close()));

const createDialog = document.getElementById("create-dialog");

document.getElementById("btn-new-user").addEventListener("click", () => {
  document.getElementById("create-form").reset();
  document.getElementById("create-msg").textContent = "";
  createDialog.showModal();
});

document.getElementById("create-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const msg = document.getElementById("create-msg");
  const password = document.getElementById("create-password").value;
  if (password !== document.getElementById("create-confirm").value) {
    msg.textContent = "las contraseñas no coinciden";
    return;
  }
  try {
    const u = await sendJSON("POST", "/api/users", {
      username: document.getElementById("create-username").value.trim(),
      password,
      role: document.getElementById("create-role").value,
    });
    createDialog.close();
    setUsersMsg(`usuario ${u.username} creado`, "ok");
    loadUsers();
  } catch (err) {
    msg.textContent = err.message;
  }
});

const deleteDialog = document.getElementById("delete-dialog");
let deleteTarget = null;

function openDeleteDialog(user) {
  deleteTarget = user;
  document.getElementById("delete-note").textContent = `¿Eliminar al usuario "${user.username}"? Se cerrarán sus sesiones y esta acción no se puede deshacer.`;
  document.getElementById("delete-msg").textContent = "";
  deleteDialog.showModal();
}

document.getElementById("delete-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  try {
    await sendJSON("DELETE", `/api/users/${deleteTarget.id}`);
    deleteDialog.close();
    setUsersMsg(`${deleteTarget.username} eliminado`, "ok");
    loadUsers();
  } catch (err) {
    document.getElementById("delete-msg").textContent = err.message;
  }
});

// Diálogo compartido: sin argumento cambia la propia contraseña (pide la
// actual); con un usuario, es el reseteo que hace un admin.
const pwDialog = document.getElementById("pw-dialog");
let pwTarget = null;

function openPasswordDialog(user = null) {
  pwTarget = user;
  const self = !user;
  document.getElementById("pw-title").textContent = self ? "Cambiar mi contraseña" : "Cambiar contraseña";
  document.getElementById("pw-note").textContent = self ? "Se cerrarán tus otras sesiones abiertas." : `Usuario: ${user.username}. Se cerrarán sus sesiones abiertas.`;
  document.getElementById("pw-current-wrap").hidden = !self;
  document.getElementById("pw-current").required = self;
  document.getElementById("pw-form").reset();
  document.getElementById("pw-msg").textContent = "";
  pwDialog.showModal();
}

document.getElementById("btn-password").addEventListener("click", () => openPasswordDialog());
document.getElementById("pw-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const msg = document.getElementById("pw-msg");
  const next = document.getElementById("pw-new").value;
  if (next !== document.getElementById("pw-confirm").value) {
    msg.textContent = "las contraseñas no coinciden";
    return;
  }
  try {
    if (pwTarget) {
      await sendJSON("PUT", `/api/users/${pwTarget.id}`, { password: next });
      setUsersMsg(`contraseña de ${pwTarget.username} actualizada`, "ok");
    } else {
      await sendJSON("PUT", "/api/auth/password", { current_password: document.getElementById("pw-current").value, new_password: next });
    }
    pwDialog.close();
  } catch (err) {
    msg.textContent = err.message;
  }
});
