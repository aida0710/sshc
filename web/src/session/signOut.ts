import { apiClient } from "../api/client";
import { clearBrowserToken, clearSessionCSRF, loadBrowserToken } from "./bootstrap";

// signOut ends this browser's session on the engine and forgets its
// registration, then drops what this page kept. The caller reloads: with
// nothing to recover from, the app shows how to enter again.
export async function signOut(): Promise<void> {
  const browserToken = loadBrowserToken();
  await apiClient.mutate("/api/v1/session/sign-out", {
    method: "POST",
    headers: browserToken === "" ? {} : { "X-SSHC-Browser": browserToken },
  });
  clearBrowserToken();
  clearSessionCSRF();
}
