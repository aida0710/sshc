import { useCallback, useEffect, useState } from "react";

// Load is what useLoad has read so far. retry reads again, for the button a failure offers.
export type Load<T> = (
  | { state: "loading" }
  | { state: "loaded"; value: T }
  | { state: "failed"; error: unknown }
) & { retry: () => void };

// useLoad reads once when the page opens, and again whenever load changes (pass it through useCallback,
// with what it reads as its dependencies). An answer that comes back after load has changed, or after the
// page has closed, is dropped, so a slow answer for an earlier target never shows for the current one.
export function useLoad<T>(load: () => Promise<T>): Load<T> {
  const [result, setResult] = useState<{ load: () => Promise<T>; outcome: { state: "loaded"; value: T } | { state: "failed"; error: unknown } } | null>(null);
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let active = true;
    setResult(null);
    load().then(
      (value) => { if (active) setResult({ load, outcome: { state: "loaded", value } }); },
      (error: unknown) => { if (active) setResult({ load, outcome: { state: "failed", error } }); },
    );
    return () => { active = false; };
  }, [load, attempt]);

  const retry = useCallback(() => setAttempt((current) => current + 1), []);

  // Until the effect has started reading for a new load, the previous answer is for another target.
  if (result === null || result.load !== load) return { state: "loading", retry };
  return { ...result.outcome, retry };
}
