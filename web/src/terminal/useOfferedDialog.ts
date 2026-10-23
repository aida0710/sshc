import { useCallback, useState } from "react";

export type OfferedDialogState = {
  // openFor is the run target the dialog is open for.
  openFor: string | null;
  // ask opens the dialog because the user chose to.
  ask: (alias: string) => void;
  // offer opens it because the run target was found in the state the dialog
  // is about, unless the user already closed it for that run target.
  offer: (alias: string) => void;
  // allowOffer lets offer open it again, once the user chooses that run
  // target anew.
  allowOffer: (alias: string) => void;
  // close closes it as declined; finish closes it because it did its job.
  close: () => void;
  finish: () => void;
};

// useOfferedDialog holds a dialog about one run target that opens by itself
// when the run target is found needing it. The session list is read every few
// seconds, so a dialog the user closed would otherwise reopen at once.
export function useOfferedDialog(): OfferedDialogState {
  const [openFor, setOpenFor] = useState<string | null>(null);
  const [declined, setDeclined] = useState(() => new Set<string>());

  const ask = useCallback((alias: string) => setOpenFor(alias), []);
  const offer = useCallback((alias: string) => {
    if (!declined.has(alias)) setOpenFor(alias);
  }, [declined]);
  // Changing the declined set also lets the caller's effect offer the dialog
  // again when the user reselects the already selected run target.
  const allowOffer = useCallback((alias: string) => {
    setDeclined((current) => {
      if (!current.has(alias)) return current;
      const allowed = new Set(current);
      allowed.delete(alias);
      return allowed;
    });
  }, []);
  const close = useCallback(() => {
    if (openFor !== null) setDeclined((current) => new Set(current).add(openFor));
    setOpenFor(null);
  }, [openFor]);
  const finish = useCallback(() => {
    if (openFor !== null) allowOffer(openFor);
    setOpenFor(null);
  }, [allowOffer, openFor]);

  return { openFor, ask, offer, allowOffer, close, finish };
}
