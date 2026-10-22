import { useCallback, useRef, useState } from "react";

export type RemoteVaultUnlockState = {
  // unlockFor is the run target whose Vault password is being asked for.
  unlockFor: string | null;
  // revision grows each time a Vault is unlocked, so that what was refused
  // while it was locked is read again.
  revision: number;
  // ask opens the password dialog because the user chose to.
  ask: (alias: string) => void;
  // offer opens it because the run target was found locked, unless the user
  // already closed it for that run target.
  offer: (alias: string) => void;
  // allowOffer lets offer open it again, once the user chooses that run
  // target anew.
  allowOffer: (alias: string) => void;
  close: () => void;
  unlocked: () => void;
};

// useRemoteVaultUnlock holds the dialog that asks for the Vault password of a
// run-target engine.
export function useRemoteVaultUnlock(): RemoteVaultUnlockState {
  const [unlockFor, setUnlockFor] = useState<string | null>(null);
  const [revision, setRevision] = useState(0);
  // declined are the run targets whose dialog the user closed. Found locked
  // again on every list, they would otherwise reopen it at once.
  const declined = useRef(new Set<string>());

  const ask = useCallback((alias: string) => setUnlockFor(alias), []);
  const offer = useCallback((alias: string) => {
    if (!declined.current.has(alias)) setUnlockFor(alias);
  }, []);
  const allowOffer = useCallback((alias: string) => { declined.current.delete(alias); }, []);
  const close = useCallback(() => {
    setUnlockFor((current) => {
      if (current !== null) declined.current.add(current);
      return null;
    });
  }, []);
  const unlocked = useCallback(() => {
    setUnlockFor((current) => {
      if (current !== null) declined.current.delete(current);
      return null;
    });
    setRevision((current) => current + 1);
  }, []);

  return { unlockFor, revision, ask, offer, allowOffer, close, unlocked };
}
