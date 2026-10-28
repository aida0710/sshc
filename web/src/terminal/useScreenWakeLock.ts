import { useEffect } from "react";
import { isScreenWakeLockSupported } from "./keepScreenAwake";

// useScreenWakeLock keeps the screen on while `active` is true. The browser lets
// go of the lock whenever the page is hidden, so it is asked for again each time
// the page is shown. A refusal (battery saver, a policy) leaves the screen to the
// device's own timeout.
export function useScreenWakeLock(active: boolean): void {
  useEffect(() => {
    if (!active || !isScreenWakeLockSupported()) return;
    let lock: WakeLockSentinel | null = null;
    let requesting = false;
    let stopped = false;
    const request = async () => {
      if (stopped || requesting || document.visibilityState !== "visible" || (lock !== null && !lock.released)) return;
      requesting = true;
      try {
        const granted = await navigator.wakeLock.request("screen");
        if (stopped) void granted.release();
        else lock = granted;
      } catch {
        // Refused: nothing to undo.
      } finally {
        requesting = false;
      }
    };
    const requestWhenShown = () => void request();
    void request();
    document.addEventListener("visibilitychange", requestWhenShown);
    return () => {
      stopped = true;
      document.removeEventListener("visibilitychange", requestWhenShown);
      void lock?.release();
    };
  }, [active]);
}
