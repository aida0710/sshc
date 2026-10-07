import type { UpdateJob, UpdateStatus } from "../api/update";
import type { MessageKey } from "../i18n/messages/en";
import { isUpdateActive } from "./useUpdateStatus";

const updateMessages: Readonly<Record<string, MessageKey>> = {
  update_unmanaged: "update.unmanaged",
  update_development_build: "update.developmentBuild",
  update_windows_unsupported: "update.windowsUnsupported",
  update_android_unsupported: "update.androidUnsupported",
  update_permission_denied: "update.permissionDenied",
  update_in_progress: "update.inProgress",
  update_plan_changed: "update.planChanged",
  action_token_expired: "update.planChanged",
  action_token_invalid: "update.planChanged",
  update_state_failed: "update.stateFailed",
  update_install_failed: "update.installFailed",
  update_interrupted: "update.interrupted",
  update_homebrew_unsupported: "update.homebrewUnsupported",
  update_restart_failed: "update.restartRequired",
  update_check_failed: "update.checkFailed",
};

export function updateMessage(code: string): MessageKey {
  return updateMessages[code] ?? "update.unavailable";
}

// The reason explains why this installation cannot update from the Web. A job that is
// running or has failed reports its own outcome, so the reason would only repeat it.
export function explainedUpdateReason({ reason, job }: UpdateStatus): string | undefined {
  if (reason === undefined || reason === "" || reason === job?.problem || isUpdateActive(job)) return undefined;
  return reason;
}

export function updateJobMessage(job: UpdateJob): MessageKey {
  switch (job.state) {
    case "succeeded": return "update.succeeded";
    case "failed":
    case "restart_required": return updateMessage(job.problem);
    case "restarting": return "update.restarting";
    default: return "update.installing";
  }
}
