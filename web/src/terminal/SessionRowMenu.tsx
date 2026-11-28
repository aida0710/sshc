import type { PaneEdge } from "./splitView/layout";
import type { RefObject } from "react";
import type { TerminalSession } from "../api/terminalSessions";
import { useTranslate } from "../i18n/context";
import { MenuGroup, MenuItem } from "../ui/MenuItems";
import { MenuPanel } from "../ui/MenuPanel";
import { splitRoomRefusalKey } from "./splitView/splitRoomRefusal";

// A workspace the session can be moved to, already named for this screen.
export type WorkspaceMoveTarget = { id: string; label: string };

// The menu of one row in the terminal list, beside its 「…」 button (anchorRef). Every item closes the menu after
// it acts.
export function SessionRowMenu({
  session,
  anchorRef,
  canDuplicate,
  canSplit = () => false,
  splitLacksRoom = () => false,
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
  anchorRef: RefObject<HTMLButtonElement | null>;
  canDuplicate: boolean;
  // canSplit は、edge の側（右か下）に分割して表示できるかである。
  canSplit?: (edge: PaneEdge) => boolean;
  // splitLacksRoom は、分割できない理由が分割表示の広さかどうかである。そのときは項目に理由を出す。
  splitLacksRoom?: (edge: PaneEdge) => boolean;
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
  const choose = (action: () => void) => () => {
    action();
    onClose();
  };
  return (
    <MenuPanel label={t("terminal.rowMenu", { title: session.title })} anchorRef={anchorRef} onClose={onClose}>
      <MenuItem onSelect={choose(onRename)}>{t("terminal.rename")}</MenuItem>
      {session.presentation?.titlePinned !== true ? null : (
        <MenuItem onSelect={choose(onUnpinTitle)}>{t("terminal.unpinTitle")}</MenuItem>
      )}
      <MenuItem disabled={!canDuplicate} onSelect={choose(onDuplicate)}>
        {t("terminal.duplicate")}
      </MenuItem>
      {onSplit === undefined ? null : (
        <>
          {(["right", "bottom"] as const).map((edge) => (
            <MenuItem key={edge} disabled={!canSplit(edge)} title={!canSplit(edge) && splitLacksRoom(edge) ? t(splitRoomRefusalKey(edge)) : undefined}
              onSelect={choose(() => onSplit(edge))}>{t(edge === "right" ? "splitView.right" : "splitView.below")}</MenuItem>
          ))}
        </>
      )}
      {onRemoveFromSplit === undefined ? null : (
        <MenuItem onSelect={choose(onRemoveFromSplit)}>{t("splitView.remove")}</MenuItem>
      )}
      <MenuItem disabled={!canMoveUp} onSelect={choose(onMoveUp)}>{t("terminal.moveUp")}</MenuItem>
      <MenuItem disabled={!canMoveDown} onSelect={choose(onMoveDown)}>{t("terminal.moveDown")}</MenuItem>
      {moveTargets.length === 0 ? null : (
        <MenuGroup heading={t("workspace.moveTo")}>
          {moveTargets.map((target) => (
            <MenuItem key={target.id} onSelect={choose(() => onMoveToWorkspace(target.id))}>
              {target.label}
            </MenuItem>
          ))}
        </MenuGroup>
      )}
    </MenuPanel>
  );
}
