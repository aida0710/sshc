import { useCallback, useEffect, useRef } from "react";
import type { ApiError } from "../api/client";
import type { OpenTerminalSessionRequest } from "../api/terminalSessions";

// vaultLockedRefusal is the refusal of a remote engine whose Vault is locked; unlocking it is what lets the session open.
const vaultLockedRefusal = "remote_engine_vault_locked";

// useOpenAfterRemoteUnlock remembers a session the remote engine refused to open, and opens it once that engine's Vault
// is unlocked. The person asked for the session; the unlock was only the way to it, and asking again would be a step
// they had already taken. Another engine chosen, or a refusal for another reason, forgets it.
export function useOpenAfterRemoteUnlock({ runTarget, refusal, vaultRevision, open }: {
  runTarget: string | null;
  // refusal is why the remote engine refuses its terminals now; null while it lists them.
  refusal: ApiError | null;
  // vaultRevision counts the remote Vault unlocks (useRemoteVaultUnlock).
  vaultRevision: number;
  open: (request: OpenTerminalSessionRequest) => Promise<unknown>;
}): { noteRefused: (request: OpenTerminalSessionRequest) => void } {
  const pending = useRef<{ target: string; request: OpenTerminalSessionRequest } | null>(null);
  const noteRefused = useCallback((request: OpenTerminalSessionRequest) => {
    if (runTarget !== null) pending.current = { target: runTarget, request };
  }, [runTarget]);

  useEffect(() => {
    const waiting = pending.current;
    if (waiting === null) return;
    if (waiting.target !== runTarget || (refusal !== null && refusal.problem?.code !== vaultLockedRefusal)) pending.current = null;
  }, [refusal, runTarget]);

  const unlocks = useRef(vaultRevision);
  useEffect(() => {
    if (vaultRevision === unlocks.current) return;
    unlocks.current = vaultRevision;
    const waiting = pending.current;
    pending.current = null;
    if (waiting !== null && waiting.target === runTarget) void open(waiting.request);
  }, [open, runTarget, vaultRevision]);

  return { noteRefused };
}
