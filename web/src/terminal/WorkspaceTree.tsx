import { useMemo, useRef, useState } from "react";
import type { HostEntry } from "../api/config";
import type { LocalShellProfile } from "../api/settings";
import type { TerminalSession } from "../api/terminalSessions";
import { defaultWorkspaceId, type TerminalWorkspace } from "../api/terminalWorkspaces";
import { useTranslate } from "../i18n/context";
import { HostPickerDialog, type LocalChoice } from "../shell/HostPickerDialog";
import { ActionMenu, type ActionMenuItem } from "../ui/ActionMenu";
import { readStoredJSON, readStoredValue, writeStoredJSON, writeStoredValue } from "../ui/browserStorage";
import { localStorageKeys } from "../ui/browserStorageKeys";
import { Icon } from "../ui/icons";
import { Notice } from "../ui/surface";
import { commandTargetsForWorkspace } from "./commands/workspaceCommandTargets";
import { WorkspaceCommandCenter } from "./commands/WorkspaceCommandCenter";
import { SessionList } from "./SessionList";
import type { TerminalSessionsState } from "./sessions";
import type { UnreadSessions } from "./terminalNotifications";
import type { TerminalWorkspacesState } from "./useTerminalWorkspaces";
import { DeleteWorkspaceDialog, WorkspaceNameDialog } from "./WorkspaceDialogs";
import { WorkspaceRow } from "./WorkspaceRow";
import { groupSessionsByWorkspace, orderWithGroupReordered, workspaceDisplayName } from "./workspaceGrouping";

type WorkspaceTreeProps = {
  workspaces: TerminalWorkspacesState;
  terminalSessions: TerminalSessionsState;
  // sessions are every session in list order.
  sessions: TerminalSession[];
  selected: string | null;
  unreadBySession?: UnreadSessions;
  onSelect: (id: string) => void;
  onDuplicate: (id: string) => void;
  onReorder: (order: string[]) => void;
  localShellProfiles?: LocalShellProfile[];
  // aliases/hosts feed the same picker SFTP uses, so a new session starts
  // from one dialog whether it is a local shell or an SSH host.
  aliases?: string[];
  hosts?: HostEntry[];
  onOpenShell: (profileId: string | undefined, workspaceId: string) => void;
  onConnect?: (alias: string, workspaceId: string) => void;
  // canBroadcast is false for an engine on an SSH host: sending a command to
  // several sessions is confirmed by an action token only this machine issues.
  canBroadcast?: boolean;
  // onUnlockRemoteVault asks for the Vault password of the run target whose
  // sessions could not be listed because it is locked.
  onUnlockRemoteVault?: () => void;
};

type NameRequest = { kind: "create" } | { kind: "rename"; workspace: TerminalWorkspace };

function readCollapsed(): Set<string> {
  const stored = readStoredJSON(localStorageKeys.collapsedTerminalWorkspaces, []);
  return new Set(Array.isArray(stored) ? stored.filter((id): id is string => typeof id === "string") : []);
}

// The sidebar's sessions, grouped by workspace. Hidden workspaces, where
// `sshc terminal create` puts what agents open, stay out of the tree unless the
// heading's menu shows them.
export function WorkspaceTree({
  workspaces,
  terminalSessions,
  sessions,
  selected,
  unreadBySession = new Set(),
  onSelect,
  onDuplicate,
  onReorder,
  localShellProfiles = [],
  aliases = [],
  hosts = [],
  onOpenShell,
  onConnect,
  canBroadcast = true,
  onUnlockRemoteVault,
}: WorkspaceTreeProps) {
  const t = useTranslate();
  const [showHidden, setShowHidden] = useState(() => readStoredValue(localStorageKeys.showHiddenTerminalWorkspaces) === "true");
  const [collapsed, setCollapsed] = useState(readCollapsed);
  const [pickerWorkspace, setPickerWorkspace] = useState<string | null>(null);
  const [nameRequest, setNameRequest] = useState<NameRequest | null>(null);
  const [deleting, setDeleting] = useState<{ workspace: TerminalWorkspace; sessionCount: number } | null>(null);
  const [broadcasting, setBroadcasting] = useState<TerminalSession[] | null>(null);
  const newSessionTrigger = useRef<HTMLButtonElement>(null);

  const groups = groupSessionsByWorkspace(workspaces.workspaces, sessions, showHidden);
  const live = sessions.filter((session) => session.exited === undefined).length;
  const atSessionLimit = terminalSessions.maxSessions > 0 && live >= terminalSessions.maxSessions;
  const canOpenSession = !terminalSessions.busy && !atSessionLimit;
  const moveTargets = workspaces.workspaces.map((workspace) => ({ id: workspace.id, label: workspaceDisplayName(t, workspace) }));
  const selectedWorkspace = sessions.find((session) => session.id === selected)?.workspaceId;
  const newSessionWorkspace = groups.some((group) => group.workspace.id === selectedWorkspace)
    ? selectedWorkspace ?? defaultWorkspaceId
    : defaultWorkspaceId;
  const localChoices = useMemo<LocalChoice[]>(() => {
    const profiles = localShellProfiles.filter((profile) => profile.id !== "default");
    return [
      { id: "default", label: t("terminal.openShell"), detail: t("terminal.localhost") },
      ...profiles.map((profile) => ({ id: profile.id, label: `${t("terminal.openShell")} · ${profile.label}`, detail: profile.path })),
    ];
  }, [localShellProfiles, t]);

  function toggleShowHidden() {
    setShowHidden((current) => {
      writeStoredValue(localStorageKeys.showHiddenTerminalWorkspaces, String(!current));
      return !current;
    });
  }

  function toggleCollapsed(id: string) {
    setCollapsed((current) => {
      const next = new Set(current);
      if (!next.delete(id)) next.add(id);
      writeStoredJSON(localStorageKeys.collapsedTerminalWorkspaces, [...next]);
      return next;
    });
  }

  function workspaceActions(workspace: TerminalWorkspace, members: TerminalSession[]): ActionMenuItem[] {
    const items: ActionMenuItem[] = [
      { label: t("workspace.rename"), onSelect: () => setNameRequest({ kind: "rename", workspace }) },
      {
        label: workspace.hidden ? t("workspace.show") : t("workspace.hide"),
        onSelect: () => void workspaces.update(workspace.id, { hidden: !workspace.hidden }),
      },
    ];
    if (canBroadcast) {
      items.push({
        label: t("workspace.broadcast"),
        disabled: !members.some((session) => session.state === "connected"),
        onSelect: () => setBroadcasting(members),
      });
    }
    if (!workspace.builtIn) {
      items.push({
        label: t("workspace.delete"),
        tone: "danger",
        onSelect: () => setDeleting({ workspace, sessionCount: members.length }),
      });
    }
    return items;
  }

  const headingActions: ActionMenuItem[] = [
    { label: showHidden ? t("workspace.hideHidden") : t("workspace.showHidden"), onSelect: toggleShowHidden },
    {
      label: t("workspace.create"),
      disabled: workspaces.maxWorkspaces > 0 && workspaces.workspaces.length >= workspaces.maxWorkspaces,
      onSelect: () => setNameRequest({ kind: "create" }),
    },
  ];

  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-center pr-1">
        <span aria-hidden="true" className="block grow px-2 text-[10px] font-bold uppercase tracking-[0.14em] text-ink-faint">
          {t("workspace.heading")}
        </span>
        <ActionMenu
          label={t("workspace.headingActions")}
          items={headingActions}
          triggerClassName="flex size-9 items-center justify-center rounded-md text-ink-muted hover:bg-hover md:size-6"
          iconClassName="size-3.5"
        />
      </div>
      {terminalSessions.problem === "" ? null : (
        <Notice tone="danger" compact>
          <span className="flex min-w-0 grow flex-col items-start gap-1.5">
            {terminalSessions.problem}
            {terminalSessions.remoteVaultLocked && onUnlockRemoteVault !== undefined ? (
              <button type="button" onClick={onUnlockRemoteVault} className="min-h-9 rounded border border-control-line px-2 text-ink hover:bg-hover md:min-h-6">
                {t("runTarget.unlockOpen")}
              </button>
            ) : null}
          </span>
        </Notice>
      )}
      {workspaces.problem === "" ? null : <Notice tone="danger" compact>{workspaces.problem}</Notice>}
      <ul aria-label={t("workspace.heading")} className="flex flex-col gap-0.5">
        {groups.map(({ workspace, sessions: members }) => {
          const name = workspaceDisplayName(t, workspace);
          const expanded = !collapsed.has(workspace.id);
          return (
            // A hidden workspace shown on request is dimmed, sessions and all.
            <li key={workspace.id} className={workspace.hidden ? "opacity-60" : undefined}>
              <WorkspaceRow
                name={name}
                sessionCount={members.length}
                hidden={workspace.hidden}
                expanded={expanded}
                unread={members.some((session) => unreadBySession.has(session.id))}
                canOpenSession={canOpenSession}
                actions={workspaceActions(workspace, members)}
                onToggle={() => toggleCollapsed(workspace.id)}
                onNewSession={() => setPickerWorkspace(workspace.id)}
              />
              {expanded ? (
                <div className="ml-3 border-l border-line pl-1">
                  <SessionList
                    sessions={members}
                    selected={selected}
                    atSessionLimit={atSessionLimit}
                    busy={terminalSessions.busy}
                    unreadBySession={unreadBySession}
                    onSelect={onSelect}
                    onClose={(id) => void terminalSessions.close(id)}
                    onRename={terminalSessions.rename}
                    onUnpinTitle={(id) => terminalSessions.unpinTitle?.(id) ?? Promise.resolve(false)}
                    onDuplicate={onDuplicate}
                    onReorder={(order) => onReorder(orderWithGroupReordered(sessions.map((session) => session.id), order))}
                    moveTargets={moveTargets}
                    onMoveToWorkspace={(id, target) => void terminalSessions.moveToWorkspace(id, target)}
                  />
                </div>
              ) : null}
            </li>
          );
        })}
      </ul>
      <button
        ref={newSessionTrigger}
        type="button"
        disabled={!canOpenSession}
        onClick={() => setPickerWorkspace(newSessionWorkspace)}
        className="flex w-full min-w-0 items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm text-ink hover:bg-hover"
      >
        <Icon name="plus" className="size-3.5" aria-hidden="true" />
        {t("terminal.newSession")}
      </button>
      {atSessionLimit ? <p className="px-2 text-xs text-ink-muted">{t("terminal.limitReached", { max: terminalSessions.maxSessions })}</p> : null}
      <HostPickerDialog
        open={pickerWorkspace !== null}
        heading={t("terminal.newSession")}
        aliases={aliases}
        hosts={hosts}
        local={localChoices}
        returnFocusRef={newSessionTrigger}
        onChoose={(alias) => {
          setPickerWorkspace(null);
          onConnect?.(alias, pickerWorkspace ?? defaultWorkspaceId);
        }}
        onChooseLocal={(id) => {
          setPickerWorkspace(null);
          onOpenShell(id === "default" ? undefined : id, pickerWorkspace ?? defaultWorkspaceId);
        }}
        onClose={() => setPickerWorkspace(null)}
      />
      {nameRequest === null ? null : (
        <WorkspaceNameDialog
          heading={nameRequest.kind === "create" ? t("workspace.create") : t("workspace.rename")}
          submitLabel={nameRequest.kind === "create" ? t("workspace.createConfirm") : t("workspace.renameConfirm")}
          initialName={nameRequest.kind === "create" ? "" : nameRequest.workspace.name}
          allowEmpty={nameRequest.kind === "rename" && nameRequest.workspace.builtIn}
          onCancel={() => setNameRequest(null)}
          onSubmit={(name) => {
            setNameRequest(null);
            if (nameRequest.kind === "create") void workspaces.create(name);
            else void workspaces.update(nameRequest.workspace.id, { name });
          }}
        />
      )}
      {deleting === null ? null : (
        <DeleteWorkspaceDialog
          name={workspaceDisplayName(t, deleting.workspace)}
          sessionCount={deleting.sessionCount}
          destinationName={moveTargets.find((target) => target.id === defaultWorkspaceId)?.label ?? ""}
          onCancel={() => setDeleting(null)}
          onDelete={() => {
            setDeleting(null);
            void workspaces.remove(deleting.workspace.id);
          }}
        />
      )}
      {broadcasting === null ? null : (
        <WorkspaceCommandCenter paneTargets={commandTargetsForWorkspace(broadcasting)} onClose={() => setBroadcasting(null)} />
      )}
    </div>
  );
}
