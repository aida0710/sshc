import { useEffect, useState } from "react";
import { syncApi } from "../api/sync";

// useSyncConfigured tells whether this machine's sync is set up: null until the status is read. It is read again
// each time the page that asks is shown, so setting sync up on the Sync page shows when the person comes back. A
// status that cannot be read counts as not set up, so a page keeps its way to Sync.
export function useSyncConfigured(visible: boolean): boolean | null {
  const [configured, setConfigured] = useState<boolean | null>(null);
  useEffect(() => {
    if (!visible) return;
    let current = true;
    syncApi.syncStatus().then(
      (status) => { if (current) setConfigured(status.configured); },
      () => { if (current) setConfigured(false); },
    );
    return () => { current = false; };
  }, [visible]);
  return configured;
}
