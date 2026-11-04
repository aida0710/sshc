import type { DragEvent } from "react";
import { dialogHeading } from "../ui/form";
import { useTranslate } from "../i18n/context";
import { ActionMenu, type ActionMenuItem } from "../ui/ActionMenu";
import { draggableCursor, dropTargetLook } from "../ui/dragAndDropLook";
import { Icon } from "../ui/icons";
import { ModalShell } from "../ui/ModalShell";
import { Button } from "../ui/surface";
import { noGroup } from "./dragdrop";
import type { GroupListing } from "./groupListing";
import type { Notice } from "../api/config";
import { GroupFolderNotices } from "./GroupFolderNotices";
import type { ConnectionDrag } from "./useConnectionDrag";

export type ScopeChoice = { kind: "all" } | { kind: "group"; name: string } | { kind: "ungrouped" };

type GroupSheetProps = {
  groups: readonly GroupListing[];
  total: number;
  ungrouped: number;
  active: ScopeChoice;
  onChoose: (scope: ScopeChoice) => void;
  // null のときは［新しいグループ］を出さない。
  onCreate: (() => void) | null;
  groupActions: (name: string) => ActionMenuItem[];
  folderNotices?: readonly Notice[];
  onDeclare?: (name: string) => void;
  onClose: () => void;
  // スマホには渡さない（ドラッグが無い）。PC では、行をこの一覧の中の別の行へ落としてグループを動かす。中に接続先の
  // 無いグループは接続の一覧に見出しが出ないので、ドラッグを始められるのはここだけである。
  drag?: ConnectionDrag;
};

// GroupSheet は、横幅の狭い画面で、グループの絞り込みとグループの操作を下から出す一覧にまとめる。
// 広い画面では左のグループの列が同じ役目を持つ。
export function GroupSheet({ groups, total, ungrouped, active, onChoose, onCreate, groupActions, folderNotices = [], onDeclare, onClose, drag }: GroupSheetProps) {
  const t = useTranslate();

  function isActive(scope: ScopeChoice): boolean {
    if (scope.kind !== active.kind) return false;
    return scope.kind !== "group" || (active.kind === "group" && active.name === scope.name);
  }

  function row(scope: ScopeChoice, label: string, count: number, depth: number, menu: ActionMenuItem[]) {
    const key = scope.kind === "group" ? `group:${scope.name}` : scope.kind;
    const selected = isActive(scope);
    const dropTarget = scope.kind === "group" ? scope.name : scope.kind === "ungrouped" ? noGroup : null;
    const receivesDrops = drag !== undefined && dropTarget !== null;
    const rowDraggable = drag !== undefined && !drag.movesDisabled && scope.kind === "group";
    return (
      <li
        key={key}
        className={`flex items-center gap-1 rounded-md ${receivesDrops ? dropTargetLook({ accepts: drag.accepts(dropTarget), over: drag.isOver(dropTarget) }) : ""}`}
        {...(receivesDrops ? drag.dropHandlers(dropTarget) : {})}
      >
        <button
          type="button"
          aria-pressed={selected}
          onClick={() => {
            onChoose(scope);
            onClose();
          }}
          {...(rowDraggable ? {
            draggable: true,
            onDragStart: (event: DragEvent) => drag.startDrag(event, { kind: "group", name: scope.name }, { floatGroupList: false }),
            onDragEnd: drag.endDrag,
          } : {})}
          style={{ paddingInlineStart: `${0.75 + depth * 1.25}rem` }}
          className={`flex min-h-11 min-w-0 flex-1 items-center gap-2 rounded-md pr-3 text-left text-sm ${selected ? "bg-select-fill font-medium text-accent" : "text-ink hover:bg-hover"} ${rowDraggable ? draggableCursor : ""}`}
        >
          <Icon name="groups" className="size-4 text-ink-faint" />
          <span title={label} className="min-w-0 flex-1 truncate">{label}</span>
          <span className="shrink-0 text-xs text-ink-faint">{count}</span>
        </button>
        {menu.length === 0 ? <span className="size-11 shrink-0" /> : (
          <ActionMenu
            label={t("tree.groupActions", { name: label })}
            items={menu}
            triggerClassName="flex size-11 items-center justify-center rounded-md text-ink-muted hover:bg-hover"
          />
        )}
      </li>
    );
  }

  return (
    <ModalShell
      labelledBy="group-sheet-heading"
      onDismiss={onClose}
      closeOnOutside
      placement="sheet"
      panelClassName="flex max-h-[80dvh] w-full max-w-md flex-col rounded-lg"
    >
      <div className="flex items-center justify-between gap-2 border-b border-line px-4 py-2">
        <h2 id="group-sheet-heading" className={dialogHeading}>{t("tree.byGroups")}</h2>
        {onCreate === null ? null : (
          <Button className="inline-flex items-center gap-1.5" onClick={onCreate}>
            <Icon name="plus" className="size-4" />
            {t("tree.newGroup")}
          </Button>
        )}
      </div>
      {onDeclare === undefined || folderNotices.length === 0 ? null : (
        <div className="px-3 pt-3">
          <GroupFolderNotices notices={folderNotices} onDeclare={onDeclare} />
        </div>
      )}
      <ul className="min-h-0 flex-1 overflow-y-auto p-2">
        {row({ kind: "all" }, t("tree.allConnections"), total, 0, [])}
        {groups.map((group) => row({ kind: "group", name: group.name }, group.label, group.count, group.depth, groupActions(group.name)))}
        {row({ kind: "ungrouped" }, t("tree.ungrouped"), ungrouped, 0, [])}
      </ul>
    </ModalShell>
  );
}
