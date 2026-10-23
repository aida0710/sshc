import { ApiError, failureCode } from "../api/client";
import type { Translate } from "../i18n/context";

// runTargetProblemText says why an engine on an SSH host could not be added,
// opened or reached, with what to do about it on that host.
export function runTargetProblemText(t: Translate, error: unknown, alias: string): string {
  const problem = error instanceof ApiError ? error.problem : null;
  switch (failureCode(error)) {
    case "remote_engine_not_running":
      return t("runTarget.notRunning", { alias });
    case "remote_engine_connect_failed":
      if (problem?.reason === "host_key_unknown") return t("runTarget.hostKeyUnknown", { alias });
      if (problem?.reason === "prompt_unavailable") return t("runTarget.promptUnavailable", { alias });
      return t("runTarget.connectFailed", { alias });
    case "remote_engine_unproven":
      return t("runTarget.unproven", { alias });
    case "remote_engine_forwarding_refused":
      return t("runTarget.forwardingRefused", { alias });
    case "remote_engine_not_installed":
      return t("runTarget.notInstalled", { alias });
    case "remote_engine_wrong_passphrase":
      return t("runTarget.wrongPassword", { alias });
    case "remote_engine_unlock_failed":
      return t("runTarget.unlockFailed", { alias });
    default:
      return t("runTarget.failed", { alias });
  }
}
