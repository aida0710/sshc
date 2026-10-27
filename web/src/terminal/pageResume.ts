// whenPageResumes calls back when the page, or the computer it runs on, comes back from being away: the
// page returns from the browser's back-forward cache (which closes every WebSocket), the network comes
// back, or the computer wakes from sleep. After a sleep a WebSocket can look open while the connection
// behind it is gone (the engine on this machine notices a dead SSH link to a run target only after its
// keepalives fail, a minute or more), and what is typed into it is lost without a word.
//
// A wake is told by the clock: a check due every few seconds that runs far later means the page did not
// run in between. A hidden page is not checked, since the browser slows its timers on purpose; a page that
// was hidden that long counts as away when it is shown again.
const wakeCheckIntervalMs = 5_000;
const asleepAfterMs = 30_000;

export function whenPageResumes(callback: () => void): () => void {
  let lastCheck = Date.now();
  let hiddenAt: number | null = document.visibilityState === "hidden" ? Date.now() : null;
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
    const now = Date.now();
    if (document.visibilityState === "hidden") {
      hiddenAt = now;
      return;
    }
    const away = hiddenAt !== null && now - hiddenAt > asleepAfterMs;
    hiddenAt = null;
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
