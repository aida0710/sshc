import type { UpdateJob, UpdateStatus } from "../api/update";
import type { MessageKey } from "../i18n/messages/en";
import { isUpdateActive } from "./useUpdateStatus";

const updateMessages: Readonly<Record<string, MessageKey>> = {
  update_unmanaged: "update.unmanaged",
  update_development_build: "update.developmentBuild",
  update_android_unsupported: "update.androidUnsupported",
  update_permission_denied: "update.permissionDenied",
  update_in_progress: "update.inProgress",
  update_plan_changed: "update.planChanged",
  action_token_expired: "update.planChanged",
  action_token_invalid: "update.planChanged",
  update_state_failed: "update.stateFailed",
  update_install_failed: "update.installFailed",
  update_interrupted: "update.interrupted",
  update_homebrew_not_updated: "update.homebrewNotUpdated",
  update_restart_failed: "update.restartRequired",
  update_check_failed: "update.checkFailed",
};

export function updateMessage(code: string): MessageKey {
  return updateMessages[code] ?? "update.unavailable";
}

// installsFromReleasePage tells that this installation cannot replace itself, as an Android app: a newer release is
// installed from its release page.
export function installsFromReleasePage(status: UpdateStatus): boolean {
  return status.reason === "update_android_unsupported";
}

// The reason explains why this installation cannot update from the Web. A job that is
// running or has failed reports its own outcome, so the reason would only repeat it.
// Installing from the release page needs no explanation until there is a release to install.
export function explainedUpdateReason(status: UpdateStatus): string | undefined {
  const { reason, job } = status;
  if (reason === undefined || reason === "" || reason === job?.problem || isUpdateActive(job)) return undefined;
  if (installsFromReleasePage(status) && status.available !== true) return undefined;
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
