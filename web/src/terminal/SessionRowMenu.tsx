import type { ReactNode, RefObject } from "react";
import type { TerminalSession } from "../api/terminalSessions";
import { useTranslate } from "../i18n/context";

export type SessionRowMenuPlacement = "up" | "down";

// The menu of one row in the terminal list. Every item closes the menu after
// it acts.
export function SessionRowMenu({
  session,
  menuRef,
  placement,
  canDuplicate,
  canMoveUp,
  canMoveDown,
  onRename,
  onUnpinTitle,
  onDuplicate,
  onMoveUp,
  onMoveDown,
  onClose,
}: {
  session: TerminalSession;
  menuRef: RefObject<HTMLDivElement | null>;
  placement: SessionRowMenuPlacement;
  canDuplicate: boolean;
  canMoveUp: boolean;
  canMoveDown: boolean;
  onRename: () => void;
  onUnpinTitle: () => void;
  onDuplicate: () => void;
  onMoveUp: () => void;
  onMoveDown: () => void;
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
      <SessionRowMenuItem disabled={!canMoveUp} onClick={choose(onMoveUp)}>{t("terminal.moveUp")}</SessionRowMenuItem>
      <SessionRowMenuItem disabled={!canMoveDown} onClick={choose(onMoveDown)}>{t("terminal.moveDown")}</SessionRowMenuItem>
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
      className="block min-h-10 w-full rounded px-2 py-1.5 text-left text-xs text-ink hover:bg-hover focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:min-h-0"
    >
      {children}
    </button>
  );
}
