import { runsInAndroidApp } from "../android/native";
import type { Translate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";

// shownHistoryPaths are the files a change names on screen. sshc's own files (under sshc/ in ~/.ssh) mean something
// where a person can open them; in the Android app nobody can, and the change's name already says what it did.
export function shownHistoryPaths(paths: readonly string[]): string {
  return (runsInAndroidApp() ? paths.filter((path) => !path.startsWith("sshc/")) : paths).join(", ");
}

type OwnFileKind = "vault" | "machine" | "settings";

// ownFileKind says what one of sshc's own files (under sshc/ in ~/.ssh) holds, or null for any other file.
function ownFileKind(path: string): OwnFileKind | null {
  if (!path.startsWith("sshc/")) return null;
  if (path === "sshc/secrets" || path.startsWith("sshc/secrets/")) return "vault";
  if (path.startsWith("sshc/machines/")) return "machine";
  return "settings";
}

const ownFileNames: Record<OwnFileKind, MessageKey> = {
  vault: "history.fileVault",
  machine: "history.fileMachine",
  settings: "history.fileSettings",
};

const restoredOwnFileLabels: Record<OwnFileKind, MessageKey> = {
  vault: "history.operation.vault",
  machine: "history.operation.machine",
  settings: "history.operation.settings",
};

// restorableFileName names a file on the button that restores it. The Android app hides sshc's own files
// (shownHistoryPaths), but a change can still be undone there, so the button says what the file holds.
export function restorableFileName(path: string, t: Translate): string {
  const kind = ownFileKind(path);
  return runsInAndroidApp() && kind !== null ? t(ownFileNames[kind]) : path;
}

// historyOperationLabelKey names a recorded change by the kind of thing it changed. A restore is recorded as
// config.restore whichever file it put back, so its file says what changed: a restored vault is not a change to the
// SSH configuration.
export function historyOperationLabelKey(operation: string, paths: readonly string[] = []): MessageKey {
  if (operation === "config.restore") {
    const kind = ownFileKind(paths[0] ?? "");
    if (kind !== null) return restoredOwnFileLabels[kind];
    if (paths[0] === "known_hosts") return "history.operation.knownHosts";
  }
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
