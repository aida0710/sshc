import { useEffect, useRef, useState, type DragEvent } from "react";
import type { TerminalForward, TerminalSession } from "../api/terminalSessions";
import { useTranslate, type Translate } from "../i18n/context";
import { ConfirmDialog } from "../ui/ConfirmDialog";
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

type SessionListProps = {
  sessions: TerminalSession[];
  selected: string | null;
  // atSessionLimit is true while the engine holds as many live sessions as it
  // allows, counted over every workspace, so nothing can be duplicated.
  atSessionLimit: boolean;
  busy: boolean;
  unreadBySession?: UnreadSessions;
  onSelect: (id: string) => void;
  onClose: (id: string) => void;
  onRename: (id: string, title: string) => Promise<boolean>;
  onUnpinTitle: (id: string) => Promise<boolean>;
  onDuplicate: (id: string) => void;
  onReorder: (order: string[]) => void;
  // moveTargets are every workspace a session can be moved to; a row leaves out
  // the one it is already in.
  moveTargets: readonly WorkspaceMoveTarget[];
  onMoveToWorkspace: (sessionId: string, workspaceId: string) => void;
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
  atSessionLimit,
  busy,
  unreadBySession = new Set(),
  onSelect,
  onClose,
  onRename,
  onUnpinTitle,
  onDuplicate,
  onReorder,
  moveTargets,
  onMoveToWorkspace,
}: SessionListProps) {
  const t = useTranslate();
  const [menuFor, setMenuFor] = useState<string | null>(null);
  const [menuPlacement, setMenuPlacement] = useState<SessionRowMenuPlacement>("down");
  const [closing, setClosing] = useState<TerminalSession | null>(null);
  const [renaming, setRenaming] = useState<string | null>(null);
  const [draft, setDraft] = useState("");
  const [dragging, setDragging] = useState<string | null>(null);
  const [dropBefore, setDropBefore] = useState<string | null>(null);
  const shiftPressed = useShiftKeyHeld();
  const menuRef = useRef<HTMLDivElement | null>(null);
  const menuTriggerRef = useRef<HTMLButtonElement | null>(null);


  useEffect(() => {
    if (menuFor !== null && !sessions.some((session) => session.id === menuFor)) setMenuFor(null);
    if (renaming !== null && !sessions.some((session) => session.id === renaming)) setRenaming(null);
  }, [sessions, menuFor, renaming]);

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
    const held = dragging;
    setDragging(null);
    setDropBefore(null);
    if (held === null || held === targetId) return;
    const order = sessions.map((session) => session.id).filter((id) => id !== held);
    const at = targetId === null ? order.length : order.indexOf(targetId);
    order.splice(at < 0 ? order.length : at, 0, held);
    onReorder(order);
  }

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
            const unread = unreadBySession.has(session.id);
            const status = sessionStatusText(t, session);
            const marker = (
              <span aria-hidden="true" className={`mt-1.5 size-1.5 shrink-0 rounded-full md:mt-1 ${sessionMarkerClass(session)}`} />
            );
            const details = (
              <>
                <p className="truncate text-xs text-ink-faint md:leading-tight">
                  {t("terminal.rowDetail", { status, destination })}
                </p>
                {(session.forwards ?? []).map((forward) => (
                  <p
                    key={`${forward.kind}:${forward.listen}:${forward.to}`}
                    className={`truncate text-xs ${forward.problem === "" ? "text-ink-faint" : "text-notice-ink"}`}
                  >
                    {forward.problem === "" ? describeForward(t, forward) : describeForwardProblem(t, forward.problem)}
                  </p>
                ))}
              </>
            );
            return (
              <li
                key={session.id}
                draggable={renaming === null}
                onDragStart={(event: DragEvent) => {
                  event.dataTransfer.setData(sessionDragMimeType, session.id);
                  event.dataTransfer.effectAllowed = "move";
                  setDragging(session.id);
                }}
                onDragEnd={() => {
                  setDragging(null);
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
                className={`relative ${dragging === session.id ? "opacity-40" : ""}`}
              >
                {dropBefore === session.id && dragging !== session.id ? (
                  <span aria-hidden="true" className="absolute inset-x-2 -top-px block h-0.5 rounded bg-accent" />
                ) : null}
                <div
                  className={`relative flex items-start gap-2 rounded-md px-2 py-1.5 transition-colors md:py-0.5 ${
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
                          if (event.key === "Enter") void commitRename(session.id);
                          if (event.key === "Escape") setRenaming(null);
                        }}
                        className="w-full rounded border border-accent bg-card px-1 py-0.5 text-sm text-ink"
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
                      className="flex min-w-0 grow items-start gap-2 text-left after:absolute after:inset-0 after:rounded-md"
                    >
                      {marker}
                      <span className="min-w-0 grow">
                        <span className={`block truncate text-sm md:leading-tight ${session.id === selected ? "font-semibold text-accent" : "text-ink"}`}>{displayTitle}</span>
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
