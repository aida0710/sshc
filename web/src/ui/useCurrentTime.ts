import { useEffect, useState } from "react";

// useCurrentTime returns the wall clock, advanced every `refreshMs` instead of
// read on each render. A time read during render changes on every render, so
// an effect that depends on it and updates a parent's state never settles.
export function useCurrentTime(refreshMs: number): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), refreshMs);
    return () => window.clearInterval(timer);
  }, [refreshMs]);
  return now;
}
