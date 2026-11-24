import { ApiError, failureCode } from "../api/client";
import type { Translate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { terminalProblemKey } from "./sessions";

// RunTargetFailure は、リモートセッションの sshc エンジンについての失敗。message は理由の1文（接続名を含む）で、
// code はエンジンが断った語。code で知らせの形を決める（動いていないときに直し方を添えるなど）。
export type RunTargetFailure = { message: string; code: string };

// runTargetFailure は、エンジンへの要求が断られた error から RunTargetFailure を作る。
export function runTargetFailure(t: Translate, error: unknown, alias: string): RunTargetFailure {
  return { message: runTargetProblemText(t, error, alias), code: failureCode(error) };
}

// runTargetFailureOfCode は、接続先での操作が終わった理由の語だけから RunTargetFailure を作る。
export function runTargetFailureOfCode(t: Translate, code: string, alias: string): RunTargetFailure {
  return { message: runTargetFailureText(t, code, alias), code };
}

// runTargetProblemText says why an engine on an SSH host could not be added,
// opened or reached, with what to do about it on that host.
export function runTargetProblemText(t: Translate, error: unknown, alias: string): string {
  if (failureCode(error) === "remote_engine_connect_failed") {
    return t(connectFailureKey(error) ?? "runTarget.connectFailed", { alias });
  }
  return runTargetFailureText(t, failureCode(error), alias);
}

// runTargetFailureText says the same for a problem code alone, as an action
// on the host reports why it ended.
export function runTargetFailureText(t: Translate, code: string, alias: string): string {
  switch (code) {
    case "remote_engine_not_running":
      return t("runTarget.notRunning", { alias });
    case "remote_engine_connect_failed":
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
    case "remote_engine_action_running":
      return t("runTarget.actionRunning", { alias });
    case "remote_engine_action_timed_out":
      return t("runTarget.actionTimedOut", { alias });
    case "remote_engine_no_action":
      return t("runTarget.actionLogLost", { alias });
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
    // 打つコマンドはパネルが RemoteEngineStartCommands で並べるので、理由の文は無い。
    case "remote_engine_not_running":
      return { heading: t("runTarget.notRunningHeading", { alias }), reason: "" };
    default:
      return { heading: unavailable, reason: runTargetProblemText(t, error, alias) };
  }
}

// runTargetRefusalCause は、一覧の行に、接続できない理由を1文で添える。どうすればよいか（打つコマンドなど）は
// ターミナルの領域のパネルが言うので、ここでは言わない。
export function runTargetRefusalCause(t: Translate, error: ApiError, alias: string): string {
  if (error.code === "remote_engine_not_running") return t("runTarget.notRunningTitle");
  return runTargetRefusalText(t, error, alias).reason;
}

// runTargetRefusalState names the state of a run target that refused in a word or two, for the run target switcher.
export function runTargetRefusalState(t: Translate, error: ApiError | null): string {
  switch (error?.code) {
    case "remote_engine_vault_locked":
      return t("runTarget.stateLocked");
    case "remote_engine_incompatible":
      return t("runTarget.stateIncompatible");
    default:
      return t("runTarget.stateUnreachable");
  }
}

// connectFailureKey names the known cause of a failed SSH connection to the
// host; null when the engine gave none.
function connectFailureKey(error: unknown): MessageKey | null {
  const reason = error instanceof ApiError ? error.problem?.reason : undefined;
  if (reason === "host_key_unknown") return "runTarget.hostKeyUnknown";
  if (reason === "host_key_changed") return "runTarget.hostKeyChanged";
  if (reason === "host_key_revoked") return "runTarget.hostKeyRevoked";
  if (reason === "prompt_unavailable") return "runTarget.promptUnavailable";
  return null;
}
