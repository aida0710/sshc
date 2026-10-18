import { sessionStorageKeys } from "../ui/browserStorageKeys";

// The reload button hands the update job to the reloaded page through the tab's
// sessionStorage, so that page shows the result once and later loads do not.

export function rememberUpdateResultToCheck(jobId: string, storage: Pick<Storage, "setItem"> = window.sessionStorage) {
  try { storage.setItem(sessionStorageKeys.updateResultToCheck, jobId); } catch { /* ignore unavailable storage */ }
}

export function readUpdateResultToCheck(storage: Pick<Storage, "getItem"> = window.sessionStorage): string | null {
  try { return storage.getItem(sessionStorageKeys.updateResultToCheck); } catch { return null; }
}

export function forgetUpdateResultToCheck(storage: Pick<Storage, "removeItem"> = window.sessionStorage) {
  try { storage.removeItem(sessionStorageKeys.updateResultToCheck); } catch { /* ignore unavailable storage */ }
}
