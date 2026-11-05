import { useEffect, useState } from "react";

// useDelayedFlag is true once `on` has stayed true for delayMs, so that what a short wait would only flash is not
// shown at all. It turns false together with `on`.
export function useDelayedFlag(on: boolean, delayMs: number): boolean {
  const [waitedLong, setWaitedLong] = useState(false);
  useEffect(() => {
    if (!on) {
      setWaitedLong(false);
      return;
    }
    const timer = window.setTimeout(() => setWaitedLong(true), delayMs);
    return () => window.clearTimeout(timer);
  }, [on, delayMs]);
  return on && waitedLong;
}
