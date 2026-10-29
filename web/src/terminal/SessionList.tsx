import type { PaneEdge } from "./splitView/layout";
import { SplitLayoutMap } from "./splitView/SplitLayoutMap";
import type { SplitGroup } from "./splitView/splitGroups";
import { useEffect, useRef, useState, type DragEvent } from "react";
import type { TerminalForward, TerminalSession } from "../api/terminalSessions";
import { useTranslate, type Translate } from "../i18n/context";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { draggableCursor } from "../ui/dragAndDropLook";
import { Icon } from "../ui/icons";
import { describeForwardProblem } from "./forwardProblem";
import { sessionDragMimeType } from "./sessionDrag";
import { SessionDestinationLine } from "./SessionDestinationLine";
import { SessionMarker } from "./SessionMarker";
import { sessionStatusText } from "./sessionStatus";
import { programStatusText, rowProgramStatus, type SeenProgramStatus } from "./programStatus";
import { SessionRowMenu, type WorkspaceMoveTarget } from "./SessionRowMenu";
import { terminalDisplayTitle } from "./terminalPresentation";
import { notificationText, type UnreadSessions } from "./terminalNotifications";
import { revealOnHoverAtPcSize } from "../ui/ActionMenu";
import { useDismissibleLayer } from "../ui/useDismissibleLayer";
import { useMenuKeyboard } from "../ui/useMenuKeyboard";
import { useShiftKeyHeld } from "../ui/useShiftKeyHeld";
import { TerminalProgressBar } from "./TerminalProgressBar";
import { isImeComposing } from "../ui/imeComposition";

type SessionListProps = {
  sessions: TerminalSession[];
  selected: string | null;
  splitGroupBySession?: ReadonlyMap<string, SplitGroup>;
  // atSessionLimit is true while the engine holds as many live sessions as it
  // allows, counted over every workspace, so nothing can be duplicated.
  atSessionLimit: boolean;
  busy: boolean;
  unreadBySession?: UnreadSessions;
  seenProgramStatus?: SeenProgramStatus;
  onSelect: (id: string) => void;
  canSplitSession?: (id: string) => boolean;
  onSplitSession?: (id: string, edge: PaneEdge) => void;
  onRemoveSessionFromSplit?: (id: string) => void;
  onClose: (id: string) => void;
  onRename: (id: string, title: string) => Promise<boolean>;
  onUnpinTitle: (id: string) => Promise<boolean>;
  onDuplicate: (id: string) => void;
  onReorder: (order: string[]) => void;
  // moveTargets are every workspace a session can be moved to; a row leaves out
  // the one it is already in.
  moveTargets: readonly WorkspaceMoveTarget[];
  onMoveToWorkspace: (sessionId: string, workspaceId: string) => void;
  // heldSessionId is the session being dragged anywhere in the tree, so a
  // list can take a session dragged out of another workspace.
  heldSessionId: string | null;
  onHoldSession: (id: string | null) => void;
  // onMoveIn takes a session from another workspace into this one, before
  // beforeId or after the last session when it is null.
  onMoveIn: (sessionId: string, beforeId: string | null) => void;
};

function describeForward(t: Translate, forward: TerminalForward): string {
  switch (forward.kind) {
    case "agent":
      return t("terminal.forwardAgent");
    case "dynamic":
      return t("terminal.forwardDynamic", { listen: forward.listen });
    case "remote":
      return t("terminal.forwardRemote", { listen: forward.listen, to: forward.to });
    default:
      return t("terminal.forwardLocal", { listen: forward.listen, to: forward.to });
  }
}

export function SessionList({
  sessions,
  selected,
  splitGroupBySession,
  atSessionLimit,
  busy,
  unreadBySession = new Set(),
  seenProgramStatus = new Map(),
  onSelect,
  canSplitSession,
  onSplitSession,
  onRemoveSessionFromSplit,
  onClose,
  onRename,
  onUnpinTitle,
  onDuplicate,
  onReorder,
  moveTargets,
  onMoveToWorkspace,
  heldSessionId,
  onHoldSession,
  onMoveIn,
}: SessionListProps) {
  const t = useTranslate();
  const [menuFor, setMenuFor] = useState<string | null>(null);
  const [closing, setClosing] = useState<TerminalSession | null>(null);
  const [renaming, setRenaming] = useState<string | null>(null);
  const [draft, setDraft] = useState("");
  const [dropBefore, setDropBefore] = useState<string | null>(null);
  const shiftPressed = useShiftKeyHeld();
  const menuRef = useRef<HTMLDivElement | null>(null);
  const menuTriggerRef = useRef<HTMLButtonElement | null>(null);


  useEffect(() => {
    if (menuFor !== null && !sessions.some((session) => session.id === menuFor)) setMenuFor(null);
    if (renaming !== null && !sessions.some((session) => session.id === renaming)) setRenaming(null);
  }, [sessions, menuFor, renaming]);

  // A drag that ended over another list, or outside the tree, leaves no mark here.
  useEffect(() => {
    if (heldSessionId === null) setDropBefore(null);
  }, [heldSessionId]);

  useDismissibleLayer({
    open: menuFor !== null,
    containerRefs: [menuRef, menuTriggerRef],
    onDismiss: () => setMenuFor(null),
    returnFocusRef: menuTriggerRef,
  });
  useMenuKeyboard({
    open: menuFor !== null,
    menuRef,
    onClose: () => setMenuFor(null),
  });

  function move(id: string, delta: number) {
    const order = sessions.map((session) => session.id);
    const from = order.indexOf(id);
    const to = from + delta;
    if (from < 0 || to < 0 || to >= order.length) return;
    order.splice(to, 0, ...order.splice(from, 1));
    onReorder(order);
  }

  function drop(targetId: string | null) {
    const held = heldSessionId;
    onHoldSession(null);
    setDropBefore(null);
    if (held === null || held === targetId) return;
    if (!sessions.some((session) => session.id === held)) {
      onMoveIn(held, targetId);
      return;
    }
    const order = sessions.map((session) => session.id).filter((id) => id !== held);
    const at = targetId === null ? order.length : order.indexOf(targetId);
    order.splice(at < 0 ? order.length : at, 0, held);
    onReorder(order);
  }

  const rowsDraggable = renaming === null;
  const rowCursor = rowsDraggable ? draggableCursor : "";

  async function commitRename(id: string) {
    const wanted = draft.trim();
    setRenaming(null);
    const current = sessions.find((session) => session.id === id)?.title ?? "";
    if (wanted === "" || wanted === current) return;
    await onRename(id, wanted);
  }

  return (
    <div className="flex flex-col gap-2">
      {sessions.length === 0 ? (
        <p className="px-2 text-xs text-ink-muted">{t("terminal.noSessions")}</p>
      ) : (
        <ul
          aria-label={t("terminal.sessionList")}
          className="flex flex-col gap-0.5"
          onDragOver={(event: DragEvent) => {
            if (event.dataTransfer.types.includes(sessionDragMimeType)) event.preventDefault();
          }}
          onDrop={(event: DragEvent) => {
            if (event.dataTransfer.types.includes(sessionDragMimeType)) drop(null);
          }}
        >
          {sessions.map((session, index) => {
            const displayTitle = terminalDisplayTitle(session);
            const splitGroup = splitGroupBySession?.get(session.id);
            const unread = unreadBySession.has(session.id);
            const programStatus = rowProgramStatus(session, seenProgramStatus);
            const status = programStatus === null ? sessionStatusText(t, session) : programStatusText(t, programStatus);
            const isSelected = session.id === selected;
            const statusId = `session-status-${session.id}`;
            const marker = <SessionMarker session={session} unread={unread} programStatus={programStatus} />;
            // The dot carries the status for the eye; screen readers get it as the row's description.
            const statusDescription = (
              <span id={statusId} className="sr-only">
                {unread ? `${status} · ${t("terminal.unreadNotification")}` : status}
              </span>
            );
            const details = (
              <>
                {unread && session.lastNotification !== undefined ? (
                  <span className="block truncate text-[11.5px] leading-4 text-notice-ink">
                    {notificationText(t, session.lastNotification)}
                  </span>
                ) : null}
                {(session.forwards ?? []).map((forward) => (
                  <p
                    key={`${forward.kind}:${forward.listen}:${forward.to}`}
                    className={`truncate text-[11px] ${forward.problem === "" ? "text-ink-faint" : "text-notice-ink"}`}
                  >
                    {forward.problem === "" ? describeForward(t, forward) : describeForwardProblem(t, forward.problem)}
                  </p>
                ))}
              </>
            );
            return (
              <li
                key={session.id}
                draggable={rowsDraggable}
                onDragStart={(event: DragEvent) => {
                  event.dataTransfer.setData(sessionDragMimeType, session.id);
                  event.dataTransfer.effectAllowed = "move";
                  onHoldSession(session.id);
                }}
                onDragEnd={() => {
                  onHoldSession(null);
                  setDropBefore(null);
                }}
                onDragOver={(event: DragEvent) => {
                  if (!event.dataTransfer.types.includes(sessionDragMimeType)) return;
                  event.preventDefault();
                  setDropBefore(session.id);
                }}
                onDrop={(event: DragEvent) => {
                  // A dragged workspace goes on to the workspace this row is in.
                  if (!event.dataTransfer.types.includes(sessionDragMimeType)) return;
                  event.stopPropagation();
                  drop(session.id);
                }}
                className={`relative ${rowCursor} ${heldSessionId === session.id ? "opacity-40" : ""}`}
              >
                {dropBefore === session.id && heldSessionId !== null && heldSessionId !== session.id ? (
                  <span aria-hidden="true" className="absolute inset-x-2 -top-px block h-0.5 rounded bg-accent" />
                ) : null}
                <div
                  className={`group/row relative flex items-start gap-2.5 rounded-[7px] py-1.5 pl-3 pr-2 transition-colors md:min-h-[30px] md:py-[3px] ${
                    shiftPressed
                      ? "bg-danger/10 hover:bg-danger/10"
                      : isSelected ? "bg-select-fill" : "hover:bg-hover"
                  }`}
                  // A right click or a long press opens the row's menu, which is the way to it where the 「…」 shows
                  // only under a pointer that the screen does not have.
                  onContextMenu={renaming === session.id ? undefined : (event) => {
                    const trigger = event.currentTarget.querySelector<HTMLButtonElement>("[data-session-menu-trigger]");
                    if (trigger === null || menuFor === session.id) return;
                    event.preventDefault();
                    trigger.click();
                  }}
                >

                  {session.programProgress !== undefined ? <TerminalProgressBar progress={session.programProgress} /> : null}
                  {renaming === session.id ? (
                    <>
                      {marker}
                      <div className="min-w-0 grow">
                      <input
                        autoFocus
                        aria-label={t("terminal.renameLabel", { title: session.title })}
                        value={draft}
                        onChange={(event) => setDraft(event.target.value)}
                        onBlur={() => void commitRename(session.id)}
                        onKeyDown={(event) => {
                          if (isImeComposing(event)) return;
                          if (event.key === "Enter") void commitRename(session.id);
                          if (event.key === "Escape") setRenaming(null);
                        }}
                        className="w-full rounded border border-accent bg-card px-1 py-0.5 text-xs text-ink"
                      />
                        {details}
                        {statusDescription}
                      </div>
                    </>
                  ) : (
                    <button
                      type="button"
                      aria-label={displayTitle}
                      aria-describedby={statusId}
                      aria-current={isSelected ? "true" : undefined}
                      onClick={() => onSelect(session.id)}
                      className={`flex min-w-0 grow items-start gap-2.5 text-left after:absolute after:inset-0 after:rounded-[7px] ${rowCursor}`}
                    >
                      {marker}
                      <span className="min-w-0 grow">
                        {/* The name gets the whole first line; the destination (or the state) is the second. */}
                        <span className="flex min-w-0 items-center gap-1.5 text-[13px] leading-[18px]">
                          <span className={`min-w-0 truncate ${isSelected ? "font-medium text-ink" : "text-ink"}`}>{displayTitle}</span>
                          {splitGroup === undefined ? null : (
                            // Where this session sits in its split, drawn small; the name is for screen readers.
                            <span data-split-group-badge={splitGroup.number} title={t("splitView.groupName", { number: splitGroup.number })}
                              aria-label={t("splitView.groupName", { number: splitGroup.number })} className="inline-flex shrink-0 items-center">
                              <SplitLayoutMap root={splitGroup.view.root} highlight={session.id} className="h-[11px] w-4" />
                            </span>
                          )}
                        </span>
                        <SessionDestinationLine session={session} programStatus={programStatus} />
                        {details}
                        {statusDescription}
                      </span>
                    </button>
                  )}
                  {/* On a PC-sized screen the name has the whole row, and the actions show only on the hovered (or
                      keyboard-reached) row, over the end of the name line. The phone layout keeps them in the row,
                      always shown. */}
                  <div className={`flex shrink-0 items-center gap-0.5 desktop-layout:absolute desktop-layout:right-1.5 desktop-layout:top-[2px] desktop-layout:rounded-md ${
                    isSelected ? "desktop-layout:bg-select-fill" : "desktop-layout:bg-hover"
                  } ${menuFor === session.id ? "" : revealOnHoverAtPcSize}`}>
                    <button
                      type="button"
                      data-session-menu-trigger
                      aria-label={t("terminal.rowMenu", { title: session.title })}
                      aria-expanded={menuFor === session.id}
                      onClick={(event) => {
                        menuTriggerRef.current = event.currentTarget;
                        if (menuFor === session.id) {
                          setMenuFor(null);
                          return;
                        }
                        setMenuFor(session.id);
                      }}
                      className="relative mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-md text-ink-muted hover:bg-hover focus:bg-select-fill focus:outline-none md:mt-0 md:size-5"
                    >
                      <Icon name="moreHorizontal" className="size-3.5" />
                    </button>
                    <button
                      type="button"
                      aria-label={t("terminal.closeSession", { title: session.title })}
                      onClick={(event) => {
                        if (session.exited !== undefined || event.shiftKey || shiftPressed) {
                          onClose(session.id);
                          return;
                        }
                        setClosing(session);
                      }}
                      className="relative mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-md text-ink-muted hover:bg-hover focus:bg-select-fill focus:outline-none md:mt-0 md:size-5"
                    >
                      <Icon name="close" className="size-3.5" />
                    </button>
                  </div>
                </div>
                {menuFor === session.id ? (
                  <SessionRowMenu
                    session={session}
                    menuRef={menuRef}
                    triggerRef={menuTriggerRef}
                    canDuplicate={!busy && !atSessionLimit}
                    canSplit={canSplitSession?.(session.id) ?? false}
                    {...(onSplitSession === undefined ? {} : { onSplit: (edge: PaneEdge) => onSplitSession(session.id, edge) })}
                    {...(onRemoveSessionFromSplit === undefined || splitGroup === undefined ? {} : { onRemoveFromSplit: () => onRemoveSessionFromSplit(session.id) })}
                    canMoveUp={index > 0}
                    canMoveDown={index < sessions.length - 1}
                    onRename={() => {
                      setDraft(session.title);
                      setRenaming(session.id);
                    }}
                    onUnpinTitle={() => void onUnpinTitle(session.id)}
                    onDuplicate={() => onDuplicate(session.id)}
                    onMoveUp={() => move(session.id, -1)}
                    onMoveDown={() => move(session.id, 1)}
                    moveTargets={moveTargets.filter((target) => target.id !== session.workspaceId)}
                    onMoveToWorkspace={(workspaceId) => onMoveToWorkspace(session.id, workspaceId)}
                    onClose={() => setMenuFor(null)}
                  />
                ) : null}
              </li>
            );
          })}
        </ul>
      )}
      {closing === null ? null : (
        <CloseConfirmation
          session={closing}
          onCancel={() => setClosing(null)}
          onConfirm={() => {
            onClose(closing.id);
            setClosing(null);
          }}
        />
      )}
    </div>
  );
}
function CloseConfirmation({
  session,
  onCancel,
  onConfirm,
}: {
  session: TerminalSession;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const t = useTranslate();
  const forwards = (session.forwards ?? []).length;
  return (
    <ConfirmDialog
      id="close-session-heading"
      heading={t("terminal.closeHeading", { title: session.title })}
      body={
        <>
          <p className="text-sm text-ink-muted">{t("terminal.closeBody")}</p>
          {forwards === 0 ? null : (
            <p className="text-sm text-ink-muted">
              {t("terminal.closeForwards", { count: String(forwards) })}
            </p>
          )}
        </>
      }
      confirmLabel={t("terminal.closeConfirm")}
      cancelLabel={t("terminal.closeCancel")}
      onConfirm={onConfirm}
      onCancel={onCancel}
    />
  );
}
