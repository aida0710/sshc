import { failureCode } from "../api/client";
import type { MessageKey } from "../i18n/messages";

// What to tell the user when a change to the transfer queue or its settings
// did not go through.
export function transferControlFailureMessage(error: unknown): MessageKey {
  const code = failureCode(error) || (error instanceof Error ? error.message : "");
  if (code === "sftp_transfer_state") return "sftp.manager.controlChanged";
  if (code === "sftp_failed" || code === "sftp_cleanup_pending") return "sftp.manager.cleanupFailed";
  return "sftp.manager.controlFailed";
}
