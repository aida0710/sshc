import { useCallback, useEffect, useState } from "react";
import type { TerminalRestoreApi, TerminalRestoreResult, TerminalRestoreSession } from "../api/terminalRestore";

export type TerminalRestoreOfferState = {
  // sessions is empty when there is nothing to ask about.
  sessions: readonly TerminalRestoreSession[];
  restore: () => Promise<TerminalRestoreResult>;
  decline: () => Promise<void>;
  // dismiss hides the question until the screen loads again; the offer stays.
  dismiss: () => void;
};

// useTerminalRestoreOffer asks the engine whether sessions from before its last
// stop wait to be opened again. Reading it is best effort: a failure only means
// nothing is asked.
export function useTerminalRestoreOffer(api: TerminalRestoreApi): TerminalRestoreOfferState {
  const [sessions, setSessions] = useState<readonly TerminalRestoreSession[]>([]);

  useEffect(() => {
    let current = true;
    api.offer().then(
      (offer) => { if (current) setSessions(offer.sessions); },
      () => undefined,
    );
    return () => { current = false; };
  }, [api]);

  const restore = useCallback(async () => {
    const result = await api.restore();
    setSessions([]);
    return result;
  }, [api]);

  const decline = useCallback(async () => {
    setSessions([]);
    await api.decline();
  }, [api]);

  const dismiss = useCallback(() => setSessions([]), []);

  return { sessions, restore, decline, dismiss };
}
