addPasswordToggles();
initAuth();

// ---------- tabs ----------

document.querySelectorAll(".nav-btn").forEach((btn) => {
  btn.addEventListener("click", () => {
    const name = btn.dataset.view;
    document.querySelectorAll(".nav-btn").forEach((b) => b.classList.toggle("active", b === btn));
    document.querySelectorAll(".view").forEach((v) => v.classList.toggle("active", v.id === `view-${name}`));
    document.getElementById("section-name").textContent = name;
    if (name === "Usuarios") loadUsers();
  });
});

function updateClock() {
  document.getElementById("clock").textContent =
    new Intl.DateTimeFormat("es-AR", { hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false }).format(new Date()) + " ART";
}
updateClock();
setInterval(updateClock, 1000);

// ---------- loop principal ----------

const dotEl = document.getElementById("dot");
const statusEl = document.getElementById("status");
let tickCount = 0;

async function tick() {
  tickCount++;
  if (tickCount === 1 || tickCount % REDISCOVER_EVERY === 0) {
    await tickSysInfo();
    await tickUptimeSessions();
  }

  let stats, alarms;
  try {
    stats = await fetchJSON("/api/stats");
  } catch {
    dotEl.style.background = "var(--crit)";
    dotEl.style.animation = "none";
    statusEl.textContent = "sin conexión con el backend";
    return;
  }
  try {
    const json = await fetchJSON("/api/alarms");
    alarms = json.alarms || [];
  } catch {
    alarms = [];
  }

  let netSpikeValues = null;
  await Promise.allSettled(
    Object.entries(METRICS).flatMap(([section, list]) =>
      list.map(async (metric) => {
        const data = metric.compute(stats);
        if (metric.history) {
          const h = await fetchHistory(metric.history);
          let values = h.values;
          if (metric.extraHistory) values = sumSeries(values, (await fetchHistory(metric.extraHistory)).values);
          data.spark = values;
          data.times = h.times;
          if (metric.key === "net-total") netSpikeValues = values;
        }
        paintMetric(cards[metric.key], metric, data);
      })
    )
  );

  for (const [section, primary] of Object.entries(PRIMARY_HISTORY)) {
    const h = await fetchHistory(primary.history);
    let values = h.values;
    if (primary.extraHistory) values = sumSeries(values, (await fetchHistory(primary.extraHistory)).values);
    sectionCharts[section].render(values, h.times, "var(--primary)", primary.fmt);
  }

  paintGeneral(stats);
  renderSockets(stats);
  renderAppsTop(stats);
  await tickContainers();
  await tickSSH();
  await tickConnections();
  await tickNetwork();

  let allAlarms = alarms;
  if (netSpikeValues && netSpikeValues.length >= 8) {
    const lastVal = netSpikeValues[netSpikeValues.length - 1];
    const rest = netSpikeValues.slice(0, -1);
    const mean = rest.reduce((a, b) => a + b, 0) / rest.length;
    const variance = rest.reduce((a, b) => a + (b - mean) ** 2, 0) / rest.length;
    const std = Math.sqrt(variance);
    if (std > 0 && lastVal > mean + 3 * std && lastVal > mean * 1.5) {
      allAlarms = [...alarms, { severity: "warn", message: "pico anómalo de red", metric: "net.spike", value: 0, since: new Date().toISOString() }];
    }
  }
  renderAlerts(allAlarms);

  dotEl.style.background = "";
  dotEl.style.animation = "";
  statusEl.textContent = `${cachedHost?.hostname || "servidor"} — ${new Date().toLocaleTimeString()}`;
}

tick();
setInterval(tick, REFRESH_MS);
