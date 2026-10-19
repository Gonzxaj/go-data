// ---------- sparklines (interactive: hover shows the value + time at that point) ----------

const SVGNS = "http://www.w3.org/2000/svg";

function createSpark(viewW, viewH, pad, withGrid) {
  const wrap = document.createElement("div");
  wrap.className = "spark-wrap";

  const svg = document.createElementNS(SVGNS, "svg");
  svg.setAttribute("viewBox", `0 0 ${viewW} ${viewH}`);
  svg.setAttribute("preserveAspectRatio", "none");
  svg.setAttribute("fill", "none");
  svg.classList.add("spark");

  if (withGrid) {
    for (const frac of [0.25, 0.5, 0.75]) {
      const gline = document.createElementNS(SVGNS, "line");
      gline.setAttribute("class", "spark-grid");
      gline.setAttribute("x1", "0"); gline.setAttribute("x2", String(viewW));
      gline.setAttribute("y1", String(viewH * frac)); gline.setAttribute("y2", String(viewH * frac));
      svg.appendChild(gline);
    }
  }

  const line = document.createElementNS(SVGNS, "polyline");
  line.setAttribute("stroke", "currentColor");
  line.setAttribute("stroke-width", "2");
  line.setAttribute("vector-effect", "non-scaling-stroke");
  line.setAttribute("points", "");
  const guide = document.createElementNS(SVGNS, "line");
  guide.setAttribute("class", "spark-guide");
  guide.setAttribute("y1", "0"); guide.setAttribute("y2", String(viewH));
  const dot = document.createElementNS(SVGNS, "circle");
  dot.setAttribute("class", "spark-dot");
  dot.setAttribute("r", "3");
  svg.append(line, guide, dot);

  const tip = document.createElement("div");
  tip.className = "spark-tip";
  const tipStrong = document.createElement("strong");
  const tipSpan = document.createElement("span");
  tip.append(tipStrong, tipSpan);

  wrap.append(svg, tip);

  let state = { values: [], times: [], fmt: (v) => num(v, 1) };

  function xy(i) {
    const { values } = state;
    const x = values.length > 1 ? (i / (values.length - 1)) * viewW : viewW / 2;
    const min = Math.min(...values), max = Math.max(...values), span = max - min || 1;
    const y = viewH - pad - ((values[i] - min) / span) * (viewH - pad * 2);
    return [x, y];
  }

  function render(values, times, stroke, fmt) {
    state = {
      values: (values || []).map((v) => (Number.isFinite(v) ? v : 0)),
      times: times || [],
      fmt: fmt || ((v) => num(v, 1)),
    };
    line.setAttribute("stroke", stroke);
    dot.setAttribute("fill", stroke);
    if (state.values.length < 2) {
      line.setAttribute("points", "");
      guide.style.display = "none";
      dot.style.display = "none";
      tip.style.display = "none";
      return;
    }
    line.setAttribute("points", state.values.map((v, i) => xy(i).join(",")).join(" "));
  }

  function onMove(evt) {
    const { values, times, fmt } = state;
    if (values.length < 2) return;
    const rect = svg.getBoundingClientRect();
    if (!rect.width) return;
    const frac = Math.max(0, Math.min(1, (evt.clientX - rect.left) / rect.width));
    const idx = Math.round(frac * (values.length - 1));
    const [x, y] = xy(idx);

    guide.setAttribute("x1", x); guide.setAttribute("x2", x); guide.style.display = "block";
    dot.setAttribute("cx", x); dot.setAttribute("cy", y); dot.style.display = "block";

    tipStrong.textContent = fmt(values[idx]);
    tipSpan.textContent = times[idx] ? new Date(times[idx] * 1000).toLocaleTimeString() : "";
    tip.style.display = "block";
    const wrapWidth = wrap.getBoundingClientRect().width || 1;
    tip.style.left = `${Math.max(4, Math.min(wrapWidth - 4, (x / viewW) * wrapWidth))}px`;
  }
  function onLeave() {
    guide.style.display = "none";
    dot.style.display = "none";
    tip.style.display = "none";
  }
  svg.addEventListener("mousemove", onMove);
  svg.addEventListener("mouseleave", onLeave);

  return { wrap, render };
}
