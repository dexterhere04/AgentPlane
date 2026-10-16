// Formatting helpers. Everything is null-safe and returns an em dash for
// missing values so tables and tiles never show "undefined".

const nf = new Intl.NumberFormat(undefined, { maximumFractionDigits: 1 });

export function esc(v) {
  if (v === null || v === undefined) return "";
  return String(v)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

export const isNum = (n) => typeof n === "number" && Number.isFinite(n);

export function fmtInt(n) {
  if (!isNum(n)) return "—";
  return Math.round(n).toLocaleString();
}

export function fmtCompact(n) {
  if (!isNum(n)) return "—";
  return new Intl.NumberFormat(undefined, { notation: "compact", maximumFractionDigits: 1 }).format(n);
}

export function fmtMoney(n, currency = "USD") {
  if (!isNum(n)) return "—";
  const digits = Math.abs(n) < 1 && n !== 0 ? 4 : 2;
  return new Intl.NumberFormat(undefined, { style: "currency", currency, minimumFractionDigits: digits, maximumFractionDigits: digits }).format(n);
}

export function fmtMs(n) {
  if (!isNum(n)) return "—";
  if (n < 1000) return `${Math.round(n)} ms`;
  return `${(n / 1000).toFixed(2)} s`;
}

/** fraction 0..1 → "12.3%" */
export function fmtRate(n, digits = 1) {
  if (!isNum(n)) return "—";
  return `${(n * 100).toFixed(digits)}%`;
}

/** already a percentage number → "12.3%" */
export function fmtPct(n, digits = 1) {
  if (!isNum(n)) return "—";
  return `${n.toFixed(digits)}%`;
}

export function fmtNum(n, digits = 1) {
  if (!isNum(n)) return "—";
  return nf.format(n);
}

export function fmtTime(ts) {
  if (!isNum(ts) && typeof ts !== "string") return "—";
  const d = new Date(ts);
  if (Number.isNaN(d.getTime())) return "—";
  return d.toLocaleTimeString(undefined, { hour12: false });
}

export function fmtDateTime(ts) {
  if (!isNum(ts) && typeof ts !== "string") return "—";
  const d = new Date(ts);
  if (Number.isNaN(d.getTime())) return "—";
  return d.toLocaleString(undefined, { year: "numeric", month: "short", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false });
}

export function relTime(ts) {
  if (!isNum(ts) && typeof ts !== "string") return "—";
  const d = new Date(ts);
  if (Number.isNaN(d.getTime())) return "—";
  const secs = Math.round((Date.now() - d.getTime()) / 1000);
  if (secs < 5) return "just now";
  if (secs < 60) return `${secs}s ago`;
  const mins = Math.round(secs / 60);
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.round(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  return `${Math.round(hrs / 24)}d ago`;
}

export function shortId(s, n = 10) {
  if (!s) return "—";
  s = String(s);
  return s.length > n * 2 ? `${s.slice(0, n)}…${s.slice(-4)}` : s;
}

export function pretty(v) {
  try {
    return JSON.stringify(v, null, 2);
  } catch {
    return String(v);
  }
}

/** Turn an SSE/backend stage slug into a readable label. */
export function humanStage(stage) {
  if (!stage) return "—";
  return String(stage).replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
}
