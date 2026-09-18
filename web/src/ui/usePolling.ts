import { useEffect, useRef } from "react";

export type PollingOptions = {
  intervalMs: number;
  // Polling stops entirely while false; flipping it back on restarts the clock.
  enabled?: boolean;
  // Most screens have nothing to show while hidden, so their ticks are
  // skipped; work that must continue in the background opts in.
  whileHidden?: boolean;
  // Run one tick as soon as polling starts instead of waiting a full interval.
  immediately?: boolean;
};

// usePolling calls `tick` on a fixed interval. A tick still in flight is not
// overlapped by the next one, and the latest `tick` is always the one called,
// so callers pass a plain closure without memoising it.
export function usePolling(tick: () => unknown, options: PollingOptions): void {
  const { intervalMs, enabled = true, whileHidden = false, immediately = false } = options;
  const latestTick = useRef(tick);
  latestTick.current = tick;

  useEffect(() => {
    if (!enabled) return;
    let inFlight = false;
    const run = () => {
      if (inFlight) return;
      if (!whileHidden && document.visibilityState === "hidden") return;
      inFlight = true;
      let outcome: unknown;
      try {
        outcome = latestTick.current();
      } catch {
        inFlight = false;
        return;
      }
      if (outcome instanceof Promise) {
        void outcome.catch(() => undefined).finally(() => { inFlight = false; });
      } else {
        inFlight = false;
      }
    };
    if (immediately) run();
    const timer = window.setInterval(run, intervalMs);
    return () => window.clearInterval(timer);
  }, [enabled, immediately, intervalMs, whileHidden]);
}
