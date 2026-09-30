import { useEffect, useRef } from "react";
import type { IsCurrentRequest } from "./useRequestGeneration";

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
// so callers pass a plain closure without memoising it. Clearing the interval
// cannot recall a tick already waiting for its answer, so each tick gets a
// check that turns false once polling stops, restarts or unmounts; a tick that
// replaces state asks it before applying the answer.
export function usePolling(tick: (isCurrent: IsCurrentRequest) => unknown, options: PollingOptions): void {
  const { intervalMs, enabled = true, whileHidden = false, immediately = false } = options;
  const latestTick = useRef(tick);
  latestTick.current = tick;

  useEffect(() => {
    if (!enabled) return;
    let stopped = false;
    const isCurrent = () => !stopped;
    let inFlight = false;
    const run = () => {
      if (inFlight) return;
      if (!whileHidden && document.visibilityState === "hidden") return;
      inFlight = true;
      let outcome: unknown;
      try {
        outcome = latestTick.current(isCurrent);
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
    return () => {
      stopped = true;
      window.clearInterval(timer);
    };
  }, [enabled, immediately, intervalMs, whileHidden]);
}
