import type { PaneEdge } from "./splitView/layout";
import { useEffect, useRef, useState, type DragEvent } from "react";
import type { TerminalForward, TerminalSession } from "../api/terminalSessions";
import { useTranslate, type Translate } from "../i18n/context";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { draggableCursor } from "../ui/dragAndDropLook";
import { Icon } from "../ui/icons";
import { describeForwardProblem } from "./forwardProblem";
import { sessionDragMimeType } from "./sessionDrag";
import { sessionMarkerClass, sessionStatusText } from "./sessionStatus";
import { SessionRowMenu, type SessionRowMenuPlacement, type WorkspaceMoveTarget } from "./SessionRowMenu";
import { terminalDisplayTitle, terminalSubtitle } from "./terminalPresentation";
import type { UnreadSessions } from "./terminalNotifications";
import { useDismissibleLayer } from "../ui/useDismissibleLayer";
import { useMenuKeyboard } from "../ui/useMenuKeyboard";
import { useShiftKeyHeld } from "../ui/useShiftKeyHeld";
import { TerminalProgressBar } from "./TerminalProgressBar";
import { isImeComposing } from "../ui/imeComposition";

type SessionListProps = {
  sessions: TerminalSession[];
  selected: string | null;
  splitGroupNumberBySession?: ReadonlyMap<string, number>;
  // atSessionLimit is true while the engine holds as many live sessions as it
  // allows, counted over every workspace, so nothing can be duplicated.
  atSessionLimit: boolean;
  busy: boolean;
  unreadBySession?: UnreadSessions;
  onSelect: (id: string) => void;
  canSplitSession?: (id: string) => boolean;
  onSplitSession?: (id: string, edge: PaneEdge) => void;
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
  splitGroupNumberBySession,
  atSessionLimit,
  busy,
  unreadBySession = new Set(),
  onSelect,
  canSplitSession,
  onSplitSession,
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
  const [menuPlacement, setMenuPlacement] = useState<SessionRowMenuPlacement>("down");
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
          onDrop={() => drop(null)}
        >
          {sessions.map((session, index) => {
            const destination = terminalSubtitle(session, t);
            const displayTitle = terminalDisplayTitle(session);
            const splitGroupNumber = splitGroupNumberBySession?.get(session.id);
            const unread = unreadBySession.has(session.id);
            const status = sessionStatusText(t, session);
            const marker = (
              <span aria-hidden="true" className={`mt-1.5 size-1.5 shrink-0 rounded-full md:mt-1 ${sessionMarkerClass(session)}`} />
            );
            const details = (
              <>
                <p className="truncate text-[11px] text-ink-faint md:leading-tight">
                  {t("terminal.rowDetail", { status, destination })}
                </p>
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
                  event.stopPropagation();
                  drop(session.id);
                }}
                className={`relative ${rowCursor} ${heldSessionId === session.id ? "opacity-40" : ""}`}
              >
                {dropBefore === session.id && heldSessionId !== null && heldSessionId !== session.id ? (
                  <span aria-hidden="true" className="absolute inset-x-2 -top-px block h-0.5 rounded bg-accent" />
                ) : null}
                <div
                  className={`relative flex items-start gap-2 rounded-md px-2 py-1.5 transition-colors md:py-1 ${
                    shiftPressed
                      ? "bg-danger/10 hover:bg-danger/10"
                      : session.id === selected ? "bg-select-fill" : "hover:bg-hover"
                  }`}
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
                      </div>
                    </>
                  ) : (
                    <button
                      type="button"
                      aria-label={displayTitle}
                      aria-current={session.id === selected ? "true" : undefined}
                      onClick={() => onSelect(session.id)}
                      className={`flex min-w-0 grow items-start gap-2 text-left after:absolute after:inset-0 after:rounded-md ${rowCursor}`}
                    >
                      {marker}
                      <span className="min-w-0 grow">
                        <span className={`flex items-center gap-1 text-xs md:leading-tight ${session.id === selected ? "font-semibold text-accent" : "text-ink"}`}>
                          <span className="truncate">{displayTitle}</span>
                          {splitGroupNumber === undefined ? null : (
                            <span data-split-group-badge={splitGroupNumber} title={t("splitView.groupName", { number: splitGroupNumber })}
                              aria-label={t("splitView.groupName", { number: splitGroupNumber })} className="inline-flex shrink-0 items-center gap-0.5 text-[10px] font-normal text-ink-muted">
                              <Icon name="splitView" className="size-3" />{splitGroupNumber}
                            </span>
                          )}
                        </span>
                        {details}
                      </span>
                    </button>
                  )}
                  {unread ? (
                    <span
                      aria-label={t("terminal.unreadNotification")}
                      title={t("terminal.unreadNotification")}
                      className="mt-2 size-2 shrink-0 rounded-full bg-accent"
                    />
                  ) : null}
                  <button
                    type="button"
                    aria-label={t("terminal.rowMenu", { title: session.title })}
                    aria-expanded={menuFor === session.id}
                    onClick={(event) => {
                      menuTriggerRef.current = event.currentTarget;
                      if (menuFor === session.id) {
                        setMenuFor(null);
                        return;
                      }
                      const trigger = event.currentTarget.getBoundingClientRect();
                      const scroll = event.currentTarget.closest("[data-navigation-scroll]")?.getBoundingClientRect();
                      const lowerEdge = scroll?.bottom ?? window.innerHeight;
                      const upperEdge = scroll?.top ?? 0;
                      const spaceBelow = lowerEdge - trigger.bottom;
                      const spaceAbove = trigger.top - upperEdge;
                      setMenuPlacement(spaceBelow < 132 && spaceAbove > spaceBelow ? "up" : "down");
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
                {menuFor === session.id ? (
                  <SessionRowMenu
                    session={session}
                    menuRef={menuRef}
                    placement={menuPlacement}
                    canDuplicate={!busy && !atSessionLimit}
                    canSplit={canSplitSession?.(session.id) ?? false}
                    {...(onSplitSession === undefined ? {} : { onSplit: (edge: PaneEdge) => onSplitSession(session.id, edge) })}
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
