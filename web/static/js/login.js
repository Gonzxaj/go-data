// Si ya hay sesión (o el login por sesión no está activo), ir directo al panel.
fetch("/api/auth/me").then((r) => { if (r.ok) location.replace("/"); }).catch(() => {});

addPasswordToggles();

const form = document.getElementById("form");
const errorEl = document.getElementById("error");
const submit = document.getElementById("submit");

form.addEventListener("submit", async (e) => {
  e.preventDefault();
  errorEl.textContent = "";
  submit.disabled = true;
  try {
    const res = await fetch("/api/auth/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username: form.username.value.trim(), password: form.password.value }),
    });
    if (res.ok) {
      location.replace("/");
      return;
    }
    const json = await res.json().catch(() => ({}));
    errorEl.textContent = json.error || "no se pudo iniciar sesión";
    form.password.value = "";
    form.password.focus();
  } catch {
    errorEl.textContent = "no se pudo conectar con el servidor";
  } finally {
    submit.disabled = false;
  }
});
