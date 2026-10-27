import { useCallback, useEffect, useState } from "react";
import type { CredentialsApi, TOTPCodeSet } from "../api/credentials";

export type TOTPCodes =
  | { state: "loading" }
  | { state: "failed"; retry: () => void }
  | { state: "ready"; codes: TOTPCodeSet; remainingSeconds: number };

// useTOTPCodes asks the engine for a saved one-time password's codes and counts down the seconds the current
// one has left, asking again when it runs out. The seed never leaves the engine; only the codes come back.
export function useTOTPCodes(name: string, api: Pick<CredentialsApi, "totpCodes">): TOTPCodes {
  const [codes, setCodes] = useState<TOTPCodeSet | null>(null);
  const [remaining, setRemaining] = useState(0);
  const [failed, setFailed] = useState(false);

  const reload = useCallback(async () => {
    try {
      const next = await api.totpCodes(name);
      setCodes(next);
      setRemaining(next.remainingSeconds);
      setFailed(false);
    } catch {
      setFailed(true);
    }
  }, [api, name]);

  useEffect(() => {
    void reload();
  }, [reload]);

  useEffect(() => {
    if (codes === null) return;
    const timer = window.setInterval(() => {
      setRemaining((current) => {
        if (current > 1) return current - 1;
        void reload();
        return 0;
      });
    }, 1000);
    return () => window.clearInterval(timer);
  }, [codes, reload]);

  if (failed) return { state: "failed", retry: () => void reload() };
  if (codes === null) return { state: "loading" };
  return { state: "ready", codes, remainingSeconds: remaining };
}
