import type { PaneEdge } from "./splitView/layout";
import type { SplitGroup } from "./splitView/splitGroups";
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
import { terminalCommandApiFor } from "./commands/commandApi";
import { WorkspaceCommandCenter } from "./commands/WorkspaceCommandCenter";
import { SessionList } from "./SessionList";
import type { TerminalSessionsState } from "./sessions";
import type { UnreadSessions } from "./terminalNotifications";
import type { SeenProgramStatus } from "./programStatus";
import type { TerminalWorkspacesState } from "./useTerminalWorkspaces";
import { DeleteWorkspaceDialog, WorkspaceNameDialog } from "./WorkspaceDialogs";
import { WorkspaceRow } from "./WorkspaceRow";
import {
  beforeIdForWorkspaceMove,
  groupSessionsByWorkspace,
  orderWithGroupReordered,
  orderWithSessionMovedInto,
  workspaceDisplayName,
  type WorkspacePlace,
} from "./workspaceGrouping";
import { useWorkspaceReorderDrag } from "./useWorkspaceReorderDrag";

type WorkspaceTreeProps = {
  workspaces: TerminalWorkspacesState;
  terminalSessions: TerminalSessionsState;
  // sessions are every session in list order.
  sessions: TerminalSession[];
  selected: string | null;
  splitGroupBySession?: ReadonlyMap<string, SplitGroup>;
  unreadBySession?: UnreadSessions;
  seenProgramStatus?: SeenProgramStatus;
  onSelect: (id: string) => void;
  canSplitSession?: (id: string) => boolean;
  onSplitSession?: (id: string, edge: PaneEdge) => void;
  onRemoveSessionFromSplit?: (id: string) => void;
  onDuplicate: (id: string) => void;
  onReorder: (order: string[]) => void;
  localShellProfiles?: LocalShellProfile[];
  // aliases/hosts feed the same picker SFTP uses, so a new session starts
  // from one dialog whether it is a local shell or an SSH host.
  aliases?: string[];
  hosts?: HostEntry[];
  onOpenShell: (profileId: string | undefined, workspaceId: string) => void;
  onConnect?: (alias: string, workspaceId: string) => void;
  // runTarget is the SSH alias of the engine the shells open on, or null for
  // this machine.
  runTarget?: string | null;
};

type NameRequest = { kind: "create" } | { kind: "rename"; workspace: TerminalWorkspace };

function readCollapsed(): Set<string> {
  const stored = readStoredJSON(localStorageKeys.collapsedTerminalWorkspaces, []);
  return new Set(Array.isArray(stored) ? stored.filter((id): id is string => typeof id === "string") : []);
}

// The sidebar's sessions, grouped by workspace in the engine's order, which a
// workspace's menu or a drag of its row changes. Hidden workspaces, where
// `sshc terminal create` puts what agents open, stay out of the tree unless the
// heading's menu shows them.
export function WorkspaceTree({
  workspaces,
  terminalSessions,
  sessions,
  selected,
  splitGroupBySession,
  unreadBySession = new Set(),
  seenProgramStatus = new Map(),
  onSelect,
  canSplitSession,
  onSplitSession,
  onRemoveSessionFromSplit,
  onDuplicate,
  onReorder,
  localShellProfiles = [],
  runTarget = null,
  aliases = [],
  hosts = [],
  onOpenShell,
  onConnect,
}: WorkspaceTreeProps) {
  const t = useTranslate();
  const [showHidden, setShowHidden] = useState(() => readStoredValue(localStorageKeys.showHiddenTerminalWorkspaces) === "true");
  const [collapsed, setCollapsed] = useState(readCollapsed);
  const [pickerWorkspace, setPickerWorkspace] = useState<string | null>(null);
  const [nameRequest, setNameRequest] = useState<NameRequest | null>(null);
  const [deleting, setDeleting] = useState<{ workspace: TerminalWorkspace; sessionCount: number } | null>(null);
  const [broadcasting, setBroadcasting] = useState<TerminalSession[] | null>(null);
  // heldSession is the session being dragged, so other workspaces can take it.
  const [heldSession, setHeldSession] = useState<string | null>(null);
  const newSessionTrigger = useRef<HTMLButtonElement>(null);

  const groups = groupSessionsByWorkspace(workspaces.workspaces, sessions, showHidden);
  // Sessions with an unread notification, in the order the sidebar shows them.
  const unreadInOrder = groups.flatMap((group) => group.sessions).filter((session) => unreadBySession.has(session.id));

  // openNextUnread opens the unread session after the selected one, going round to the first,
  // and opens its workspace if it is folded.
  function openNextUnread() {
    const after = unreadInOrder.findIndex((session) => session.id === selected) + 1;
    const next = unreadInOrder[after % unreadInOrder.length];
    if (next === undefined) return;
    if (collapsed.has(next.workspaceId)) toggleCollapsed(next.workspaceId);
    onSelect(next.id);
  }
  const live = sessions.filter((session) => session.exited === undefined).length;
  const atSessionLimit = terminalSessions.maxSessions > 0 && live >= terminalSessions.maxSessions;
  const canOpenSession = !terminalSessions.busy && !atSessionLimit;
  const moveTargets = workspaces.workspaces.map((workspace) => ({ id: workspace.id, label: workspaceDisplayName(t, workspace) }));
  const selectedWorkspace = sessions.find((session) => session.id === selected)?.workspaceId;
  const newSessionWorkspace = groups.some((group) => group.workspace.id === selectedWorkspace)
    ? selectedWorkspace ?? defaultWorkspaceId
    : defaultWorkspaceId;
  const heldSessionWorkspace = heldSession === null
    ? null
    : groups.find((group) => group.sessions.some((session) => session.id === heldSession))?.workspace.id ?? null;

  function moveWorkspace(workspaceId: string, place: WorkspacePlace) {
    const allWorkspaceIds = workspaces.workspaces.map((workspace) => workspace.id);
    void workspaces.move(workspaceId, beforeIdForWorkspaceMove(allWorkspaceIds, workspaceId, place));
  }
  const workspaceDrag = useWorkspaceReorderDrag(moveWorkspace);

  function moveSessionInto(workspaceId: string, members: readonly TerminalSession[], sessionId: string, beforeId: string | null) {
    setHeldSession(null);
    onReorder(orderWithSessionMovedInto(sessions.map((session) => session.id), {
      sessionId,
      members: members.map((session) => session.id),
      beforeId,
    }));
    void terminalSessions.moveToWorkspace(sessionId, workspaceId);
  }

  const localChoices = useMemo<LocalChoice[]>(() => {
    const profiles = localShellProfiles.filter((profile) => profile.id !== "default");
    return [
      { id: "default", label: t("terminal.openShell"), detail: runTarget ?? t("terminal.localhost") },
      ...profiles.map((profile) => ({ id: profile.id, label: `${t("terminal.openShell")} · ${profile.label}`, detail: profile.path })),
    ];
  }, [localShellProfiles, runTarget, t]);

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

  // Moving up and down steps over the workspaces the tree shows; hidden ones left out of it keep their places.
  function workspaceActions(workspace: TerminalWorkspace, members: TerminalSession[], index: number): ActionMenuItem[] {
    const above = groups[index - 1]?.workspace.id;
    const below = groups[index + 1]?.workspace.id;
    const items: ActionMenuItem[] = [
      { label: t("workspace.rename"), onSelect: () => setNameRequest({ kind: "rename", workspace }) },
      {
        label: workspace.hidden ? t("workspace.show") : t("workspace.hide"),
        onSelect: () => void workspaces.update(workspace.id, { hidden: !workspace.hidden }),
      },
      {
        label: t("terminal.moveUp"),
        disabled: above === undefined,
        onSelect: () => {
          if (above !== undefined) moveWorkspace(workspace.id, { targetId: above, side: "before" });
        },
      },
      {
        label: t("terminal.moveDown"),
        disabled: below === undefined,
        onSelect: () => {
          if (below !== undefined) moveWorkspace(workspace.id, { targetId: below, side: "after" });
        },
      },
    ];
    // The tree shows the sessions of one run target, so a broadcast stays within it.
    items.push({
      label: t("workspace.broadcast"),
      disabled: !members.some((session) => session.state === "connected"),
      onSelect: () => setBroadcasting(members),
    });
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
  ];

  return (
    <div className="flex flex-col gap-0.5">
      {/* The heading stays at the top of the sidebar's list while the sessions scroll under it, so its
          buttons are always in reach. On a phone they are 44px touch targets, which already space it from
          the first workspace. */}
      <div className="sticky top-0 z-[2] mt-3.5 flex items-center bg-sidebar pr-1 pt-1 md:pb-2.5">
        <span aria-hidden="true" className="block grow px-2 text-xs text-ink-faint">
          {t("workspace.heading")}
        </span>
        {unreadInOrder.length === 0 ? null : (
          <button
            type="button"
            title={t("terminal.openNextUnread")}
            onClick={openNextUnread}
            className="inline-flex min-h-9 shrink-0 items-center gap-1 rounded-full bg-notice px-1.5 text-[11px] text-notice-ink hover:brightness-110 md:min-h-5"
          >
            <span aria-hidden="true" className="size-1.5 rounded-full bg-notice-ink" />
            {t("terminal.unreadCount", { count: String(unreadInOrder.length) })}
          </button>
        )}
        <button
          type="button"
          aria-label={t("workspace.create")}
          title={t("workspace.create")}
          disabled={workspaces.maxWorkspaces > 0 && workspaces.workspaces.length >= workspaces.maxWorkspaces}
          onClick={() => setNameRequest({ kind: "create" })}
          className="flex size-9 shrink-0 items-center justify-center rounded-md text-ink-muted hover:bg-hover md:size-5"
        >
          <Icon name="plus" className="size-3.5" />
        </button>
        <ActionMenu
          label={t("workspace.headingActions")}
          items={headingActions}
          triggerClassName="flex size-9 items-center justify-center rounded-md text-ink-muted hover:bg-hover md:size-5"
          iconClassName="size-3.5"
        />
      </div>
      {terminalSessions.problem === "" ? null : <Notice tone="danger" compact>{terminalSessions.problem}</Notice>}
      {workspaces.problem === "" ? null : <Notice tone="danger" compact>{workspaces.problem}</Notice>}
      <ul aria-label={t("workspace.heading")} className="flex flex-col gap-0.5">
        {groups.map(({ workspace, sessions: members }, index) => {
          const name = workspaceDisplayName(t, workspace);
          const expanded = !collapsed.has(workspace.id);
          const held = workspaceDrag.heldWorkspaceId === workspace.id;
          const dropSide = workspaceDrag.dropPlace?.targetId === workspace.id && !held ? workspaceDrag.dropPlace.side : null;
          return (
            // A hidden workspace shown on request is dimmed, sessions and all.
            <li
              key={workspace.id}
              {...workspaceDrag.dropTargetProps(workspace.id)}
              className={`relative not-first:mt-1.5 ${held ? "opacity-40" : workspace.hidden ? "opacity-60" : ""}`}
            >
              {/* The line sits in the gap between workspaces, where the dragged one would go. */}
              {dropSide === null ? null : (
                <span aria-hidden="true" className={`absolute inset-x-2 z-[2] block h-0.5 rounded bg-accent ${dropSide === "before" ? "-top-1" : "-bottom-1"}`} />
              )}
              {/* Like the group headings of the connection list, a workspace's name stays under the Workspaces
                  heading while its sessions scroll by, and gives way to the next workspace's. The offset is the
                  heading's height (48px on a phone, where its buttons are 44px touch targets). */}
              <div className="sticky top-12 z-[1] bg-sidebar md:top-[34px]">
              <WorkspaceRow
                name={name}
                sessionCount={members.length}
                hidden={workspace.hidden}
                expanded={expanded}
                unread={members.some((session) => unreadBySession.has(session.id))}
                canOpenSession={canOpenSession}
                actions={workspaceActions(workspace, members, index)}
                onToggle={() => toggleCollapsed(workspace.id)}
                onNewSession={() => setPickerWorkspace(workspace.id)}
                acceptsSession={heldSessionWorkspace !== null && heldSessionWorkspace !== workspace.id}
                onDropSession={(id) => moveSessionInto(workspace.id, members, id, null)}
                dragSourceProps={workspaceDrag.dragSourceProps(workspace.id)}
              />
              </div>
              {expanded ? (
                <div className="pl-1">
                  <SessionList
                    sessions={members}
                    selected={selected}
                    {...(splitGroupBySession === undefined ? {} : { splitGroupBySession })}
                    atSessionLimit={atSessionLimit}
                    busy={terminalSessions.busy}
                    unreadBySession={unreadBySession}
                    seenProgramStatus={seenProgramStatus}
                    onSelect={onSelect}
                    {...(canSplitSession === undefined ? {} : { canSplitSession })}
                    {...(onSplitSession === undefined ? {} : { onSplitSession })}
                    {...(onRemoveSessionFromSplit === undefined ? {} : { onRemoveSessionFromSplit })}
                    onClose={(id) => void terminalSessions.close(id)}
                    onRename={terminalSessions.rename}
                    onUnpinTitle={(id) => terminalSessions.unpinTitle?.(id) ?? Promise.resolve(false)}
                    onDuplicate={onDuplicate}
                    onReorder={(order) => onReorder(orderWithGroupReordered(sessions.map((session) => session.id), order))}
                    moveTargets={moveTargets}
                    onMoveToWorkspace={(id, target) => void terminalSessions.moveToWorkspace(id, target)}
                    heldSessionId={heldSession}
                    onHoldSession={setHeldSession}
                    onMoveIn={(id, beforeId) => moveSessionInto(workspace.id, members, id, beforeId)}
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
        className="mt-1.5 flex min-h-10 w-full min-w-0 items-center gap-2.5 rounded-[7px] px-2 text-left text-[13px] text-accent hover:bg-hover md:min-h-[30px]"
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
        <WorkspaceCommandCenter paneTargets={commandTargetsForWorkspace(broadcasting)} api={terminalCommandApiFor(runTarget)} onClose={() => setBroadcasting(null)} />
      )}
    </div>
  );
}
