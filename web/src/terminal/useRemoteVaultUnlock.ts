import { useCallback, useState } from "react";
import { useOfferedDialog } from "./useOfferedDialog";

export type RemoteVaultUnlockState = {
  // unlockFor is the run target whose Vault password is being asked for.
  unlockFor: string | null;
  // revision grows each time a Vault is unlocked, so that what was refused
  // while it was locked is read again.
  revision: number;
  ask: (alias: string) => void;
  offer: (alias: string) => void;
  allowOffer: (alias: string) => void;
  close: () => void;
  unlocked: () => void;
};

// useRemoteVaultUnlock holds the dialog that asks for the Vault password of a
// run-target engine.
export function useRemoteVaultUnlock(): RemoteVaultUnlockState {
  const dialog = useOfferedDialog();
  const [revision, setRevision] = useState(0);
  const { finish } = dialog;
  const unlocked = useCallback(() => {
    finish();
    setRevision((current) => current + 1);
  }, [finish]);

  return {
    unlockFor: dialog.openFor, revision, ask: dialog.ask, offer: dialog.offer, allowOffer: dialog.allowOffer, close: dialog.close, unlocked,
  };
}
