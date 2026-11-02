import { useEffect, useState, type DragEvent } from "react";
import { useTranslate } from "../i18n/context";
import { ActionMenu, revealOnHover, type ActionMenuItem } from "../ui/ActionMenu";
import { DisclosureChevron } from "../ui/DisclosureChevron";
import { draggableCursor, dropTargetLook } from "../ui/dragAndDropLook";
import { Icon } from "../ui/icons";
import { sessionDragMimeType } from "./sessionDrag";

// One workspace in the sidebar tree: opens and closes its sessions, starts a
// session in it, holds its own actions, takes a session dragged onto it from
// another workspace, and is the handle to drag the workspace itself by.
export function WorkspaceRow({
  name,
  sessionCount,
  hidden,
  expanded,
  unread,
  canOpenSession,
  actions,
  onToggle,
  onNewSession,
  acceptsSession,
  onDropSession,
  dragSourceProps,
}: {
  name: string;
  sessionCount: number;
  hidden: boolean;
  expanded: boolean;
  unread: boolean;
  canOpenSession: boolean;
  actions: readonly ActionMenuItem[];
  onToggle: () => void;
  onNewSession: () => void;
  // acceptsSession is true while a session from another workspace is dragged.
  acceptsSession: boolean;
  onDropSession: (sessionId: string) => void;
  // dragSourceProps start the drag of this workspace, to move it in the tree.
  dragSourceProps: { draggable: boolean; onDragStart: (event: DragEvent) => void; onDragEnd: () => void };
}) {
  const t = useTranslate();
  const [over, setOver] = useState(false);

  useEffect(() => {
    if (!acceptsSession) setOver(false);
  }, [acceptsSession]);

  function dragOver(event: DragEvent) {
    if (!acceptsSession || !event.dataTransfer.types.includes(sessionDragMimeType)) return;
    event.preventDefault();
    event.dataTransfer.dropEffect = "move";
    setOver(true);
  }

  function dragLeave(event: DragEvent) {
    // Moving onto a button inside the row is not leaving it.
    if (event.relatedTarget instanceof Node && event.currentTarget.contains(event.relatedTarget)) return;
    setOver(false);
  }

  function drop(event: DragEvent) {
    setOver(false);
    if (!acceptsSession) return;
    const sessionId = event.dataTransfer.getData(sessionDragMimeType);
    if (sessionId === "") return;
    event.preventDefault();
    onDropSession(sessionId);
  }

  return (
    <div
      {...dragSourceProps}
      onDragOver={dragOver}
      onDragLeave={dragLeave}
      onDrop={drop}
      className={`group/row flex items-center gap-1 rounded-md px-1 ${draggableCursor} ${dropTargetLook({ accepts: acceptsSession, over })}`}
    >
      <button
        type="button"
        data-touch-dense
        aria-expanded={expanded}
        onClick={onToggle}
        className={`flex min-h-9 min-w-0 grow items-center gap-1 rounded-md px-0.5 text-left text-sm hover:bg-hover md:min-h-[26px] ${draggableCursor}`}
      >
        <DisclosureChevron expanded={expanded} className="size-[13px] shrink-0 text-ink-faint" />
        {/* The workspace name labels its sessions, so it stays quieter than they are. */}
        <span className="min-w-0 grow truncate text-xs font-normal text-ink-faint">{name}</span>
        {hidden ? <span className="sr-only">{t("workspace.hiddenMark")}</span> : null}
        {unread && !expanded ? (
          <span aria-label={t("terminal.unreadNotification")} className="size-2 shrink-0 rounded-full bg-notice-ink" />
        ) : null}
        <span className={`shrink-0 text-[11px] text-ink-faint ${revealOnHover}`}>{sessionCount}</span>
      </button>
      <button
        type="button"
        data-touch-dense
        aria-label={t("workspace.newSessionIn", { name })}
        title={t("workspace.newSessionIn", { name })}
        disabled={!canOpenSession}
        onClick={onNewSession}
        className={`flex size-9 shrink-0 items-center justify-center rounded-md text-ink-muted hover:bg-hover md:size-5 ${revealOnHover}`}
      >
        <Icon name="plus" className="size-3.5" />
      </button>
      <ActionMenu
        label={t("workspace.actions", { name })}
        items={actions}
        triggerClassName={`flex size-9 items-center justify-center rounded-md text-ink-muted hover:bg-hover md:size-5 ${revealOnHover}`}
        triggerTouchDense
        iconClassName="size-3.5"
      />
    </div>
  );
}
