import { useCallback, useSyncExternalStore } from "react";
import { useBeforeUnloadWarning } from "../ui/useBeforeUnloadWarning";
import { allTransferManagers, transferManagerFor } from "./transferManager";

function anyBrowserTransfers(): boolean {
  return allTransferManagers().some((manager) => manager.hasBrowserTransfers());
}

// Asks before the page closes while it runs a transfer or holds the File of a
// waiting upload. The File lives only in this page; after a reload the upload
// resumes only once the user chooses the same file again.
//
// Every engine's queue counts: a transfer goes on while another engine is shown. shownRunTarget is the engine shown
// now, whose queue is made if this is the first time, so that it is watched from the start.
export function useTransferUnloadWarning(shownRunTarget: string | null): void {
  const subscribe = useCallback((changed: () => void) => {
    transferManagerFor(shownRunTarget);
    const stops = allTransferManagers().map((manager) => manager.subscribe(changed));
    return () => stops.forEach((stop) => stop());
  }, [shownRunTarget]);
  useBeforeUnloadWarning(useSyncExternalStore(subscribe, anyBrowserTransfers));
}
