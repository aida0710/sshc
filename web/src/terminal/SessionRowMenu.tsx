import type { PaneEdge } from "./splitView/layout";
import type { ReactNode, RefObject } from "react";
import type { TerminalSession } from "../api/terminalSessions";
import { useTranslate } from "../i18n/context";

export type SessionRowMenuPlacement = "up" | "down";

// A workspace the session can be moved to, already named for this screen.
export type WorkspaceMoveTarget = { id: string; label: string };

// The menu of one row in the terminal list. Every item closes the menu after
// it acts.
export function SessionRowMenu({
  session,
  menuRef,
  placement,
  canDuplicate,
  canSplit = false,
  onSplit,
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
  placement: SessionRowMenuPlacement;
  canDuplicate: boolean;
  canSplit?: boolean;
  onSplit?: (edge: PaneEdge) => void;
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
  const choose = (action: () => void) => () => {
    action();
    onClose();
  };
  return (
    <div
      ref={menuRef}
      role="menu"
      aria-label={t("terminal.rowMenu", { title: session.title })}
      className={`absolute right-1 z-10 w-48 rounded-lg border border-control-line bg-card p-1 shadow-lg ${placement === "up" ? "bottom-full mb-0.5" : "mt-0.5"}`}
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
      <SessionRowMenuItem disabled={!canMoveUp} onClick={choose(onMoveUp)}>{t("terminal.moveUp")}</SessionRowMenuItem>
      <SessionRowMenuItem disabled={!canMoveDown} onClick={choose(onMoveDown)}>{t("terminal.moveDown")}</SessionRowMenuItem>
      {moveTargets.length === 0 ? null : (
        <div role="group" aria-label={t("workspace.moveTo")} className="mt-1 border-t border-line pt-1">
          <p aria-hidden="true" className="px-2 py-1 text-[11px] text-ink-faint">{t("workspace.moveTo")}</p>
          {moveTargets.map((target) => (
            <SessionRowMenuItem key={target.id} onClick={choose(() => onMoveToWorkspace(target.id))}>
              {target.label}
            </SessionRowMenuItem>
          ))}
        </div>
      )}
    </div>
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
      className="block min-h-10 w-full rounded px-2 py-1.5 text-left text-xs text-ink hover:bg-hover focus:bg-select-fill focus:outline-none md:min-h-0"
    >
      {children}
    </button>
  );
}
