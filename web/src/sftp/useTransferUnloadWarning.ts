import { useSftpTransferManager } from "./useSftpTransferManager";
import { useSyncExternalStore } from "react";
import { useBeforeUnloadWarning } from "../ui/useBeforeUnloadWarning";

// Asks before the page closes while it runs a transfer or holds the File of a
// waiting upload. The File lives only in this page; after a reload the upload
// resumes only once the user chooses the same file again.
export function useTransferUnloadWarning(): void {
  const sftpTransferManager = useSftpTransferManager();
  useBeforeUnloadWarning(useSyncExternalStore(sftpTransferManager.subscribe, sftpTransferManager.hasBrowserTransfers));
}
