// Auto-refresh helper. Drives the dashboards that used to be manual-refresh
// only (Overview, Analytics, Routing). A self-rescheduling timeout avoids
// overlapping requests; polling pauses while the tab is hidden and resumes
// (with an immediate tick) when it becomes visible again.

/**
 * @param {() => void | Promise<void>} fn  the work to run on each tick
 * @param {number} ms                       interval between ticks
 * @param {{immediate?: boolean}} [opts]
 * @returns {() => void} stop function
 */
export function createPoller(fn, ms, { immediate = true } = {}) {
  let timer = null;
  let stopped = false;

  const schedule = (delay) => {
    if (stopped) return;
    timer = setTimeout(run, delay);
  };

  const run = async () => {
    if (stopped) return;
    if (typeof document !== "undefined" && document.hidden) { schedule(ms); return; }
    try { await fn(); } catch { /* a failed tick must not kill the poller */ }
    schedule(ms);
  };

  const onVisibility = () => {
    if (stopped || document.hidden) return;
    if (timer) { clearTimeout(timer); timer = null; }
    run();
  };

  document.addEventListener("visibilitychange", onVisibility);

  if (immediate) run();
  else schedule(ms);

  return () => {
    stopped = true;
    if (timer) { clearTimeout(timer); timer = null; }
    document.removeEventListener("visibilitychange", onVisibility);
  };
}
