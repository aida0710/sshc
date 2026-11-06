import type { Translate } from "../i18n/context";
import type { LocalChoice } from "../shell/HostPickerDialog";

// localShellChoice is the one local shell the picker of a new session offers. Which shell it opens is the
// engine's to decide: the one chosen in Settings > Terminal, or the system login shell when none is.
// runTarget is the SSH alias of the engine the shell opens on, or null for this machine.
export function localShellChoice(t: Translate, runTarget: string | null): LocalChoice {
  return { id: localShellChoiceId, label: t("terminal.openShell"), detail: runTarget ?? t("terminal.localhost") };
}

export const localShellChoiceId = "local-shell";

// shellRequest opens that shell in a workspace.
export function shellRequest(workspaceId: string) {
  return { kind: "shell" as const, workspaceId };
}
