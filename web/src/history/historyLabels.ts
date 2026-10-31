import type { MessageKey } from "../i18n/messages";

// historyOperationLabelKey names a recorded change by the kind of thing it changed.
export function historyOperationLabelKey(operation: string): MessageKey {
  if (operation.startsWith("connection.")) return "history.operation.connection";
  if (operation.startsWith("key.")) return "history.operation.key";
  if (operation.startsWith("terminal.")) return "history.operation.terminal";
  if (operation.startsWith("settings.") || operation.startsWith("metadata.")) return "history.operation.settings";
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
