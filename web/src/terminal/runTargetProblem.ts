import { ApiError, failureCode } from "../api/client";
import type { Translate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { terminalProblemKey } from "./sessions";

// runTargetProblemText says why an engine on an SSH host could not be added,
// opened or reached, with what to do about it on that host.
export function runTargetProblemText(t: Translate, error: unknown, alias: string): string {
  switch (failureCode(error)) {
    case "remote_engine_not_running":
      return t("runTarget.notRunning", { alias });
    case "remote_engine_connect_failed":
      return t(connectFailureKey(error) ?? "runTarget.connectFailed", { alias });
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

// runTargetRefusalText says why the run target's engine refused to list its
// sessions: a heading naming what happened, and the reason or what to do when
// the heading does not already say it. Each heading names only what the error
// code shows; an unreachable engine does not say whether SSH or the engine
// failed, so its heading does not either.
export function runTargetRefusalText(t: Translate, error: ApiError, alias: string): { heading: string; reason: string } {
  const unavailable = t("runTarget.unavailableHeading", { alias });
  switch (error.code) {
    case "remote_engine_vault_locked":
      return { heading: t("runTarget.lockedHeading", { alias }), reason: t("runTarget.lockedReason", { alias }) };
    case "remote_engine_incompatible":
      return { heading: t("runTarget.incompatibleHeading", { alias }), reason: t("runTarget.incompatibleReason", { alias }) };
    case "remote_engine_unreachable":
    case "remote_engine_refused":
      return { heading: unavailable, reason: t(terminalProblemKey(error.code)) };
    case "remote_engine_connect_failed": {
      const key = connectFailureKey(error);
      return { heading: t("runTarget.connectFailedHeading", { alias }), reason: key === null ? "" : t(key, { alias }) };
    }
    default:
      return { heading: unavailable, reason: runTargetProblemText(t, error, alias) };
  }
}

// connectFailureKey names the known cause of a failed SSH connection to the
// host; null when the engine gave none.
function connectFailureKey(error: unknown): MessageKey | null {
  const reason = error instanceof ApiError ? error.problem?.reason : undefined;
  if (reason === "host_key_unknown") return "runTarget.hostKeyUnknown";
  if (reason === "prompt_unavailable") return "runTarget.promptUnavailable";
  return null;
}
