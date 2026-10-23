import { useEffect, useState } from "react";

// The count is shown in whole seconds, so it only needs to change that often.
const elapsedTickMs = 1_000;

// useElapsedSeconds counts the whole seconds since `running` became true, and
// stops counting when it becomes false.
export function useElapsedSeconds(running: boolean): number {
  const [elapsed, setElapsed] = useState(0);
  useEffect(() => {
    if (!running) return;
    const startedAt = Date.now();
    setElapsed(0);
    const timer = window.setInterval(() => setElapsed(Math.floor((Date.now() - startedAt) / 1_000)), elapsedTickMs);
    return () => window.clearInterval(timer);
  }, [running]);
  return elapsed;
}
