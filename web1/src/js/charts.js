// Hand-rolled SVG/CSS chart primitives. No chart library — these render with
// Oat's CSS variables so they inherit the theme in light and dark.

import { esc, fmtInt, fmtTime } from "./format.js";

const PALETTE = [
  "var(--primary)",
  "var(--success)",
  "var(--warning)",
  "var(--danger)",
  "#8b5cf6",
  "#0ea5e9",
  "#f59e0b",
  "#14b8a6",
  "#ec4899",
  "#64748b",
];

export const color = (i) => PALETTE[i % PALETTE.length];

/** Horizontal bar list. rows: [{label, value, text?, alt?}] */
export function barList(rows, { empty = "No data" } = {}) {
  const clean = (rows || []).filter((r) => r && Number.isFinite(r.value));
  if (!clean.length) return `<div class="chart-empty">${esc(empty)}</div>`;
  const max = Math.max(...clean.map((r) => r.value), 1);
  return `<div class="barlist">${clean
    .map((r) => {
      const pct = Math.max(2, Math.round((r.value / max) * 100));
      const text = r.text ?? fmtInt(r.value);
      return `<div class="barlist-row">
        <span class="truncate" title="${esc(r.label)}">${esc(r.label)}</span>
        <span class="barlist-track"><span class="barlist-fill${r.alt ? " alt" : ""}" style="width:${pct}%"></span></span>
        <span class="barlist-value">${esc(text)}</span>
      </div>`;
    })
    .join("")}</div>`;
}

/** Grouped latency bars: rows: [{label, p50, p95, p99}] */
export function latencyBars(rows, { empty = "No latency data" } = {}) {
  const clean = (rows || []).filter((r) => r && (Number.isFinite(r.p50) || Number.isFinite(r.p95) || Number.isFinite(r.p99)));
  if (!clean.length) return `<div class="chart-empty">${esc(empty)}</div>`;
  const max = Math.max(...clean.flatMap((r) => [r.p50, r.p95, r.p99].filter(Number.isFinite)), 1);
  const series = [
    { key: "p50", label: "p50", color: "var(--success)" },
    { key: "p95", label: "p95", color: "var(--warning)" },
    { key: "p99", label: "p99", color: "var(--danger)" },
  ];
  const legend = `<div class="hstack" style="gap:var(--space-4);font-size:var(--text-8);margin-bottom:var(--space-3)">
    ${series.map((s) => `<span><span class="swatch" style="background:${s.color}"></span>${s.label}</span>`).join("")}
  </div>`;
  const body = clean
    .map((r) => {
      const bars = series
        .filter((s) => Number.isFinite(r[s.key]))
        .map((s) => `<span class="barlist-track" style="height:0.35rem;flex:1">
            <span class="barlist-fill" style="width:${Math.max(2, (r[s.key] / max) * 100)}%;background:${s.color}"></span>
          </span>`)
        .join("");
      return `<div class="barlist-row" style="grid-template-columns:minmax(6rem,12rem) 1fr auto;align-items:center">
        <span class="truncate" title="${esc(r.label)}">${esc(r.label)}</span>
        <span class="hstack" style="gap:var(--space-2);flex-wrap:nowrap">${bars}</span>
        <span class="barlist-value">p95 ${esc(fmtInt(r.p95))}ms</span>
      </div>`;
    })
    .join("");
  return legend + `<div class="barlist">${body}</div>`;
}

/** Donut. segments: [{label, value}] → svg + legend markup. */
export function donut(segments, { size = 128, thickness = 14, empty = "No data", valueFmt = fmtInt } = {}) {
  const clean = (segments || []).filter((s) => s && s.value > 0);
  if (!clean.length) return `<div class="chart-empty">${esc(empty)}</div>`;
  const total = clean.reduce((a, s) => a + s.value, 0) || 1;

  let offset = 25; // start at 12 o'clock
  const arcs = clean
    .map((s, i) => {
      const pct = (s.value / total) * 100;
      const seg = `<circle cx="21" cy="21" r="15.9155" fill="none"
        stroke="${color(i)}" stroke-width="${thickness}"
        stroke-dasharray="${pct.toFixed(3)} ${(100 - pct).toFixed(3)}"
        stroke-dashoffset="${offset.toFixed(3)}"></circle>`;
      offset = (offset - pct + 100) % 100;
      return seg;
    })
    .join("");

  const svg = `<svg viewBox="0 0 42 42" width="${size}" height="${size}" role="img" aria-label="Distribution">
      <circle cx="21" cy="21" r="15.9155" fill="none" stroke="var(--muted)" stroke-width="${thickness}"></circle>
      ${arcs}
      <text x="21" y="20.5" text-anchor="middle" style="font-size:5px;fill:var(--muted-foreground)">total</text>
      <text x="21" y="26" text-anchor="middle" style="font-size:6.5px;font-weight:600;fill:var(--foreground)">${esc(valueFmt(total))}</text>
    </svg>`;

  const legend = `<div class="donut-legend">${clean
    .map((s, i) => `<span><span class="swatch" style="background:${color(i)}"></span>${esc(s.label)} <span class="text-light">· ${esc(valueFmt(s.value))}</span></span>`)
    .join("")}</div>`;

  return `<div class="donut-wrap">${svg}${legend}</div>`;
}

/**
 * Concurrency Gantt. rows: [{ id, label, start, end, tone, status }]
 * Bars are positioned as a % of the shared [t0, t1] window.
 */
export function gantt(rows, { empty = "No requests yet", now = Date.now() } = {}) {
  const clean = (rows || []).filter((r) => r && Number.isFinite(r.start));
  if (!clean.length) return `<div class="chart-empty">${esc(empty)}</div>`;

  const t0 = Math.min(...clean.map((r) => r.start));
  const t1 = Math.max(...clean.map((r) => (Number.isFinite(r.end) ? r.end : now)), now);
  const span = Math.max(t1 - t0, 1);

  const lanes = clean
    .map((r) => {
      const end = Number.isFinite(r.end) ? r.end : now;
      const left = ((r.start - t0) / span) * 100;
      const width = Math.max(((end - r.start) / span) * 100, 0.6);
      const meta = r.meta ? `<span class="lane-meta-text truncate">${esc(r.meta)}</span>` : "";
      return `<div class="lane">
        <div class="lane-label">
          <span class="lane-dot ${r.tone || ""}"></span>
          <span class="mono-id">${esc(r.label)}</span>
          ${meta}
        </div>
        <div class="lane-track">
          <div class="lane-bar ${r.tone || ""}" style="left:${left.toFixed(2)}%;width:${width.toFixed(2)}%"></div>
        </div>
      </div>`;
    })
    .join("");

  const axis = `<div class="lane-axis"><span>${esc(fmtTime(t0))}</span><span>${esc(fmtTime(t1))}</span></div>`;
  return `<div class="gantt">${lanes}${axis}</div>`;
}

let chartSeq = 0;
let mountScheduled = false;
const chartRegistry = new Map();

const num = (v) => { const n = Number(v); return Number.isFinite(n) ? n : 0; };

function niceCeil(v) {
  if (!(v > 0)) return 1;
  const p = Math.pow(10, Math.floor(Math.log10(v)));
  const n = v / p;
  const step = n <= 1 ? 1 : n <= 2 ? 2 : n <= 2.5 ? 2.5 : n <= 5 ? 5 : 10;
  return step * p;
}

/**
 * Register a responsive time-axis chart and return its placeholder markup.
 * The SVG is rendered at the host's measured pixel width (crisp text), then
 * re-rendered on resize. Config:
 *   { buckets:[{timestamp}], series:[{label,accessor,kind:'bar'|'line'|'area',
 *     color,axis:'left'|'right',stack,fmt}], height, xFormat, leftFormat,
 *     rightFormat, empty }
 */
export function timeSeries(cfg) {
  const id = `ts${++chartSeq}`;
  chartRegistry.set(id, cfg || {});
  if (!mountScheduled && typeof requestAnimationFrame === "function") {
    mountScheduled = true;
    requestAnimationFrame(() => { mountScheduled = false; mountCharts(document); });
  }
  return `<div class="tschart" data-chart-id="${id}"></div>`;
}

/** Render every registered chart whose host is present; prune stale ones. */
export function mountCharts(root = document) {
  const live = new Set();
  for (const host of root.querySelectorAll("[data-chart-id]")) {
    const id = host.dataset.chartId;
    const cfg = chartRegistry.get(id);
    if (!cfg) continue;
    live.add(id);
    const width = Math.max(320, Math.round(host.clientWidth || 900));
    host.innerHTML = buildTimeSeries(cfg, width);
  }
  for (const id of [...chartRegistry.keys()]) if (!live.has(id)) chartRegistry.delete(id);
}

function buildTimeSeries(cfg, W) {
  const {
    buckets = [], series = [], height = 200,
    xFormat = (t) => fmtTime(t),
    leftFormat = (n) => fmtInt(n),
    rightFormat = (n) => fmtInt(n),
    empty = "No data in this window",
  } = cfg;

  const rows = (buckets || []).filter((b) => b && Number.isFinite(new Date(b.timestamp).getTime()));
  if (!rows.length || !series.length) return `<div class="chart-empty">${esc(empty)}</div>`;

  const H = height;
  const hasRight = series.some((s) => s.axis === "right");
  const padL = 52, padR = hasRight ? 52 : 16, padT = 14, padB = 30;
  const innerW = Math.max(40, W - padL - padR);
  const innerH = H - padT - padB;
  const n = rows.length;

  const xAt = (i) => padL + (n === 1 ? innerW / 2 : (i / (n - 1)) * innerW);
  const valsOf = (s) => rows.map((b) => num(s.accessor(b)));

  const stackTotals = { left: new Map(), right: new Map() };
  for (const s of series) {
    if (s.kind !== "bar" || !s.stack) continue;
    const axis = s.axis === "right" ? "right" : "left";
    const arr = stackTotals[axis].get(s.stack) || new Array(n).fill(0);
    const v = valsOf(s);
    for (let i = 0; i < n; i++) arr[i] += Math.max(0, v[i]);
    stackTotals[axis].set(s.stack, arr);
  }

  const axisMax = (axis) => {
    let m = 0;
    for (const s of series) {
      const a = s.axis === "right" ? "right" : "left";
      if (a !== axis || (s.kind === "bar" && s.stack)) continue;
      for (const v of valsOf(s)) m = Math.max(m, Math.abs(v));
    }
    for (const arr of stackTotals[axis].values()) for (const v of arr) m = Math.max(m, v);
    return niceCeil(m);
  };
  const maxLeft = axisMax("left") || 1;
  const maxRight = hasRight ? (axisMax("right") || 1) : 1;

  const yAt = (v, axis) => padT + innerH - (Math.max(0, v) / (axis === "right" ? maxRight : maxLeft)) * innerH;
  const fmtAxis = (v, axis) => (axis === "right" ? rightFormat(v) : leftFormat(v));

  const id = `tsd${chartSeq}`;
  const defs = [], grid = [], yLabels = [];
  for (let t = 0; t <= 4; t++) {
    const frac = t / 4;
    const y = padT + innerH - frac * innerH;
    grid.push(`<line class="ts-grid" x1="${padL}" y1="${y.toFixed(1)}" x2="${(padL + innerW).toFixed(1)}" y2="${y.toFixed(1)}"/>`);
    yLabels.push(`<text class="ts-ylab" x="${padL - 6}" y="${(y + 3).toFixed(1)}" text-anchor="end">${esc(fmtAxis(maxLeft * frac, "left"))}</text>`);
    if (hasRight) yLabels.push(`<text class="ts-ylab" x="${(padL + innerW + 6).toFixed(1)}" y="${(y + 3).toFixed(1)}" text-anchor="start">${esc(fmtAxis(maxRight * frac, "right"))}</text>`);
  }

  const tickEvery = Math.max(1, Math.ceil(n / Math.max(2, Math.floor(innerW / 110))));
  const xLabels = [];
  for (let i = 0; i < n; i++) {
    if (i % tickEvery !== 0 && i !== n - 1) continue;
    const anchor = i === 0 ? "start" : i === n - 1 ? "end" : "middle";
    xLabels.push(`<text class="ts-xlab" x="${xAt(i).toFixed(1)}" y="${H - 10}" text-anchor="${anchor}">${esc(xFormat(rows[i].timestamp))}</text>`);
  }

  const barW = Math.max(1, (innerW / Math.max(n, 1)) * 0.62);
  const barRunning = { left: new Map(), right: new Map() };
  const bars = [];
  series.forEach((s, si) => {
    if (s.kind !== "bar") return;
    const axis = s.axis === "right" ? "right" : "left";
    const v = valsOf(s);
    const fill = s.color || color(si);
    for (let i = 0; i < n; i++) {
      const base = s.stack ? (barRunning[axis].get(s.stack)?.[i] || 0) : 0;
      const top = base + Math.max(0, v[i]);
      if (s.stack) {
        const arr = barRunning[axis].get(s.stack) || new Array(n).fill(0);
        arr[i] = top;
        barRunning[axis].set(s.stack, arr);
      }
      const yTop = yAt(top, axis), yBot = yAt(base, axis), h = yBot - yTop;
      if (h < 0.4) continue;
      bars.push(`<rect class="ts-bar" x="${(xAt(i) - barW / 2).toFixed(1)}" y="${yTop.toFixed(1)}" width="${barW.toFixed(1)}" height="${h.toFixed(1)}" rx="1" fill="${fill}"/>`);
    }
  });

  const lines = [];
  series.forEach((s, si) => {
    if (s.kind === "bar") return;
    const axis = s.axis === "right" ? "right" : "left";
    const c = s.color || color(si);
    const pts = valsOf(s).map((val, i) => `${xAt(i).toFixed(1)},${yAt(val, axis).toFixed(1)}`).join(" ");
    if (s.kind === "area") {
      const base = padT + innerH;
      defs.push(`<linearGradient id="${id}g${si}" x1="0" y1="0" x2="0" y2="1"><stop offset="0%" stop-color="${c}" stop-opacity="0.18"/><stop offset="100%" stop-color="${c}" stop-opacity="0"/></linearGradient>`);
      lines.push(`<polygon class="ts-area" points="${padL},${base.toFixed(1)} ${pts} ${(padL + innerW).toFixed(1)},${base.toFixed(1)}" fill="url(#${id}g${si})"/>`);
    }
    lines.push(`<polyline class="ts-line" points="${pts}" fill="none" stroke="${c}"/>`);
  });

  const colW = Math.max(innerW / Math.max(n, 1), 1);
  const hover = rows.map((b, i) => {
    const tip = [xFormat(b.timestamp), ...series.map((s) => `${s.label}: ${(s.fmt || fmtInt)(num(s.accessor(b)))}`)].join("\n");
    return `<rect class="ts-hover" x="${(xAt(i) - colW / 2).toFixed(1)}" y="${padT}" width="${colW.toFixed(1)}" height="${innerH}"><title>${esc(tip)}</title></rect>`;
  }).join("");

  const legend = `<div class="ts-legend">${series.map((s, i) => `<span class="lg"><i style="background:${s.color || color(i)}"></i>${esc(s.label)}</span>`).join("")}</div>`;

  return `${legend}<div class="chart tschart"><svg viewBox="0 0 ${W} ${H}" width="${W}" height="${H}" role="img" aria-label="Time series">
    <defs>${defs.join("")}</defs>
    ${grid.join("")}
    ${bars.join("")}
    ${lines.join("")}
    <line class="ts-axis" x1="${padL}" y1="${(padT + innerH).toFixed(1)}" x2="${(padL + innerW).toFixed(1)}" y2="${(padT + innerH).toFixed(1)}"/>
    ${yLabels.join("")}${xLabels.join("")}${hover}
  </svg></div>`;
}

/** Sparkline. values: number[] */
export function sparkline(values, { width = 240, height = 48 } = {}) {
  const clean = (values || []).filter(Number.isFinite);
  if (clean.length < 2) return `<div class="chart-empty">Not enough points</div>`;
  const max = Math.max(...clean);
  const min = Math.min(...clean);
  const span = max - min || 1;
  const stepX = width / (clean.length - 1);
  const y = (v) => height - 4 - ((v - min) / span) * (height - 8);
  const pts = clean.map((v, i) => `${(i * stepX).toFixed(1)},${y(v).toFixed(1)}`);
  const line = pts.join(" ");
  const area = `0,${height} ${line} ${width},${height}`;
  return `<div class="chart"><svg viewBox="0 0 ${width} ${height}" preserveAspectRatio="none" height="${height}">
      <polygon class="area" points="${area}"></polygon>
      <polyline class="line" points="${line}"></polyline>
    </svg></div>`;
}
