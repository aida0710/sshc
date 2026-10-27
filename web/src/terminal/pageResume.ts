// whenPageResumes calls back when the page, or the computer it runs on, comes back from being away: the
// page returns from the browser's back-forward cache (which closes every WebSocket), the network comes
// back, or the computer wakes from sleep. After a sleep a WebSocket can look open while the connection
// behind it is gone (the engine on this machine notices a dead SSH link to a run target only after its
// keepalives fail, a minute or more), and what is typed into it is lost without a word.
//
// A wake is told by the clock: a check due every few seconds that runs far later means the page did not
// run in between. A hidden page's timers are slowed on purpose (Chrome runs them once a minute after a
// while), so a page shown again counts as away only when its timers stopped for longer than that.
const wakeCheckIntervalMs = 5_000;
const asleepAfterMs = 30_000;
const hiddenAsleepAfterMs = 2 * 60_000;

export function whenPageResumes(callback: () => void): () => void {
  let lastCheck = Date.now();
  const check = () => {
    const now = Date.now();
    const late = now - lastCheck > asleepAfterMs;
    lastCheck = now;
    if (late && document.visibilityState === "visible") callback();
  };
  const timer = window.setInterval(check, wakeCheckIntervalMs);
  const pageShown = (event: PageTransitionEvent) => {
    lastCheck = Date.now();
    if (event.persisted) callback();
  };
  const visibilityChanged = () => {
    if (document.visibilityState !== "visible") return;
    const now = Date.now();
    const away = now - lastCheck > hiddenAsleepAfterMs;
    lastCheck = now;
    if (away) callback();
  };
  window.addEventListener("pageshow", pageShown);
  window.addEventListener("online", callback);
  document.addEventListener("visibilitychange", visibilityChanged);
  return () => {
    window.clearInterval(timer);
    window.removeEventListener("pageshow", pageShown);
    window.removeEventListener("online", callback);
    document.removeEventListener("visibilitychange", visibilityChanged);
  };
}
