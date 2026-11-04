import { runsInAndroidApp } from "../android/native";
import type { Translate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";

// shownHistoryPaths are the files a change names on screen. sshc's own files (under sshc/ in ~/.ssh) mean something
// where a person can open them; in the Android app nobody can, and the change's name already says what it did.
export function shownHistoryPaths(paths: readonly string[]): string {
  return (runsInAndroidApp() ? paths.filter((path) => !path.startsWith("sshc/")) : paths).join(", ");
}

// restorableFileName names a file on the button that restores it. The Android app hides sshc's own files
// (shownHistoryPaths), but a change can still be undone there, so the button says what the file holds.
export function restorableFileName(path: string, t: Translate): string {
  if (!runsInAndroidApp() || !path.startsWith("sshc/")) return path;
  if (path === "sshc/secrets" || path.startsWith("sshc/secrets/")) return t("history.fileVault");
  if (path.startsWith("sshc/machines/")) return t("history.fileMachine");
  return t("history.fileSettings");
}

// historyOperationLabelKey names a recorded change by the kind of thing it changed.
export function historyOperationLabelKey(operation: string): MessageKey {
  if (operation.startsWith("connection.")) return "history.operation.connection";
  if (operation.startsWith("key.")) return "history.operation.key";
  if (operation.startsWith("terminal.")) return "history.operation.terminal";
  if (["settings.", "metadata.", "shortcuts.", "keybar."].some((prefix) => operation.startsWith(prefix))) return "history.operation.settings";
  if (operation.startsWith("host.")) return "history.operation.hostDetection";
  if (operation.startsWith("vpn.")) return "history.operation.vpn";
  if (operation.startsWith("known_hosts.")) return "history.operation.knownHosts";
  if (operation.startsWith("sftp.")) return "history.operation.sftp";
  if (operation.startsWith("machine.")) return "history.operation.machine";
  if (operation.startsWith("secret.")) return "history.operation.vault";
  if (operation.startsWith("sync.")) return "history.operation.sync";
  if (operation.startsWith("config.") || operation.startsWith("directory.") || operation.startsWith("group.")) {
    return "history.operation.configuration";
  }
  return "history.operation.other";
}

export function historyStatusLabelKey(status: string): MessageKey {
  switch (status) {
    case "staging": return "history.status.staging";
    case "staged": return "history.status.staged";
    case "applied": return "history.status.applied";
    case "completed": return "history.status.completed";
    case "rolled_back": return "history.status.rolledBack";
    default: return "history.status.unknown";
  }
}
