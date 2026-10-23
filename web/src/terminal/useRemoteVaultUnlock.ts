import { useCallback, useState } from "react";

export type RemoteVaultUnlockState = {
  // unlockFor is the run target whose Vault password the dialog asks for.
  unlockFor: string | null;
  // revision grows each time a Vault is unlocked, so that what was refused
  // while it was locked is read again.
  revision: number;
  ask: (alias: string) => void;
  close: () => void;
  // unlocked is called once a Vault is unlocked, from the dialog or from the
  // terminal area.
  unlocked: () => void;
};

// useRemoteVaultUnlock holds the dialog that asks for the Vault password of a
// run-target engine, and counts the unlocks.
export function useRemoteVaultUnlock(): RemoteVaultUnlockState {
  const [unlockFor, setUnlockFor] = useState<string | null>(null);
  const [revision, setRevision] = useState(0);
  const close = useCallback(() => setUnlockFor(null), []);
  const unlocked = useCallback(() => {
    setUnlockFor(null);
    setRevision((current) => current + 1);
  }, []);

  return { unlockFor, revision, ask: setUnlockFor, close, unlocked };
}
