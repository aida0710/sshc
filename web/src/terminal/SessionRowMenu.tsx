import type { PaneEdge } from "./splitView/layout";
import type { ReactNode, RefObject } from "react";
import { createPortal } from "react-dom";
import type { TerminalSession } from "../api/terminalSessions";
import { useTranslate } from "../i18n/context";
import { menuGroupClass, menuGroupHeadingClass, menuItemClass, menuSurface } from "../ui/menuStyle";
import { useAnchoredMenu } from "../ui/useAnchoredMenu";

// A workspace the session can be moved to, already named for this screen.
export type WorkspaceMoveTarget = { id: string; label: string };

// The menu of one row in the terminal list. Every item closes the menu after
// it acts. It is portaled and placed beside its 「…」 button (triggerRef), so the
// list's scrolling and the phone's sliding drawer neither clip nor move it.
export function SessionRowMenu({
  session,
  menuRef,
  triggerRef,
  canDuplicate,
  canSplit = false,
  onSplit,
  onRemoveFromSplit,
  canMoveUp,
  canMoveDown,
  onRename,
  onUnpinTitle,
  onDuplicate,
  onMoveUp,
  onMoveDown,
  moveTargets,
  onMoveToWorkspace,
  onClose,
}: {
  session: TerminalSession;
  menuRef: RefObject<HTMLDivElement | null>;
  triggerRef: RefObject<HTMLButtonElement | null>;
  canDuplicate: boolean;
  canSplit?: boolean;
  onSplit?: (edge: PaneEdge) => void;
  // onRemoveFromSplit is offered while the session is in a split. The session goes on outside it.
  onRemoveFromSplit?: () => void;
  canMoveUp: boolean;
  canMoveDown: boolean;
  onRename: () => void;
  onUnpinTitle: () => void;
  onDuplicate: () => void;
  onMoveUp: () => void;
  onMoveDown: () => void;
  moveTargets: readonly WorkspaceMoveTarget[];
  onMoveToWorkspace: (workspaceId: string) => void;
  onClose: () => void;
}) {
  const t = useTranslate();
  useAnchoredMenu({ open: true, anchorRef: triggerRef, menuRef });
  const choose = (action: () => void) => () => {
    action();
    onClose();
  };
  return createPortal(
    <div
      ref={menuRef}
      role="menu"
      aria-label={t("terminal.rowMenu", { title: session.title })}
      className={`fixed z-50 w-60 overflow-y-auto ${menuSurface}`}
    >
      <SessionRowMenuItem onClick={choose(onRename)}>{t("terminal.rename")}</SessionRowMenuItem>
      {session.presentation?.titlePinned !== true ? null : (
        <SessionRowMenuItem onClick={choose(onUnpinTitle)}>{t("terminal.unpinTitle")}</SessionRowMenuItem>
      )}
      <SessionRowMenuItem disabled={!canDuplicate} onClick={choose(onDuplicate)}>
        {t("terminal.duplicate")}
      </SessionRowMenuItem>
      {onSplit === undefined ? null : (
        <>
          <SessionRowMenuItem disabled={!canSplit} onClick={choose(() => onSplit("right"))}>{t("splitView.right")}</SessionRowMenuItem>
          <SessionRowMenuItem disabled={!canSplit} onClick={choose(() => onSplit("bottom"))}>{t("splitView.below")}</SessionRowMenuItem>
        </>
      )}
      {onRemoveFromSplit === undefined ? null : (
        <SessionRowMenuItem onClick={choose(onRemoveFromSplit)}>{t("splitView.remove")}</SessionRowMenuItem>
      )}
      <SessionRowMenuItem disabled={!canMoveUp} onClick={choose(onMoveUp)}>{t("terminal.moveUp")}</SessionRowMenuItem>
      <SessionRowMenuItem disabled={!canMoveDown} onClick={choose(onMoveDown)}>{t("terminal.moveDown")}</SessionRowMenuItem>
      {moveTargets.length === 0 ? null : (
        <div role="group" aria-label={t("workspace.moveTo")} className={menuGroupClass}>
          <p aria-hidden="true" className={menuGroupHeadingClass}>{t("workspace.moveTo")}</p>
          {moveTargets.map((target) => (
            <SessionRowMenuItem key={target.id} onClick={choose(() => onMoveToWorkspace(target.id))}>
              {target.label}
            </SessionRowMenuItem>
          ))}
        </div>
      )}
    </div>,
    document.body,
  );
}

function SessionRowMenuItem({
  disabled = false,
  onClick,
  children,
}: {
  disabled?: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      role="menuitem"
      disabled={disabled}
      onClick={onClick}
      className={menuItemClass()}
    >
      {children}
    </button>
  );
}
