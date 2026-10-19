// Ojito para ver/ocultar la contraseña en cada input[type=password].
function addPasswordToggles(root = document) {
  root.querySelectorAll('input[type="password"]').forEach((input) => {
    const dialog = input.closest("dialog");
    const wrap = document.createElement("span");
    wrap.className = "pw-wrap";
    input.replaceWith(wrap);
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "pw-toggle";
    const sync = () => {
      const shown = input.type === "text";
      btn.setAttribute("aria-label", shown ? "Ocultar contraseña" : "Mostrar contraseña");
      btn.setAttribute("aria-pressed", String(shown));
      btn.title = btn.getAttribute("aria-label");
      btn.innerHTML = `<svg viewBox="0 0 24 24" aria-hidden="true">${shown ? '<path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19M14.12 14.12a3 3 0 1 1-4.24-4.24M1 1l22 22"/>' : '<path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/>'}</svg>`;
    };
    btn.addEventListener("click", () => {
      input.type = input.type === "password" ? "text" : "password";
      sync();
      input.focus();
    });
    // Al cerrar un modal la contraseña vuelve a quedar oculta.
    dialog?.addEventListener("close", () => { input.type = "password"; sync(); });
    sync();
    wrap.append(input, btn);
  });
}
