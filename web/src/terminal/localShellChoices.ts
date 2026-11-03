import type { LocalShellProfile } from "../api/settings";
import type { Translate } from "../i18n/context";
import type { LocalChoice } from "../shell/HostPickerDialog";

// localShellChoices are the shells a new session can open, one per profile, for the picker of a new session.
// runTarget is the SSH alias of the engine the shells open on, or null for this machine.
export function localShellChoices(t: Translate, profiles: readonly LocalShellProfile[], runTarget: string | null): LocalChoice[] {
  return [
    { id: "default", label: t("terminal.openShell"), detail: runTarget ?? t("terminal.localhost") },
    ...profiles.filter((profile) => profile.id !== "default")
      .map((profile) => ({ id: profile.id, label: `${t("terminal.openShell")} · ${profile.label}`, detail: profile.path })),
  ];
}

// shellRequest opens the shell of a choice above in a workspace.
export function shellRequest(choiceId: string, workspaceId: string) {
  return { kind: "shell" as const, workspaceId, ...(choiceId === "default" ? {} : { profileId: choiceId }) };
}
