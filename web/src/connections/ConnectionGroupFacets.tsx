import { Fragment, useMemo, useState, type ReactNode } from "react";
import type { Notice, Overview } from "../api/config";
import { useTranslate } from "../i18n/context";
import { ActionMenu, hideOnHoverAtPcSize, revealOnHoverAtPcSize, type ActionMenuItem } from "../ui/ActionMenu";
import { ColumnResizeHandle } from "../ui/ColumnResizeHandle";
import { DisclosureChevron } from "../ui/DisclosureChevron";
import { draggableCursor, dropTargetLook } from "../ui/dragAndDropLook";
import { Icon } from "../ui/icons";
import { localStorageKeys } from "../ui/browserStorageKeys";
import { useColumnWidth, type ColumnWidth } from "../ui/useColumnWidth";
import type { Scope } from "./ConnectionTree";
import { buildGroupTree, decorateHosts, type GroupNode } from "./connectionTreeIndex";
import type { ConnectionDrag } from "./useConnectionDrag";
import { GroupFolderNotices } from "./GroupFolderNotices";

// 220px は Settings のページの一覧の幅。この列は同じ位置に置く。
const connectionGroupsWidth: ColumnWidth = { key: localStorageKeys.connectionGroupsWidth, fallback: 220, minimum: 112, maximum: 400 };

// 入れ子のグループは、1段ごとにこれだけ右へずらす。
const depthIndent = "0.875rem";

function facetLook(active: boolean): string {
  return `flex min-h-[30px] w-full min-w-0 items-center gap-2.5 rounded-[7px] px-2 text-left text-[13px] transition-colors ${
    active ? "bg-select-fill font-medium text-ink" : "text-ink-muted hover:bg-hover hover:text-ink"
  }`;
}

// ConnectionGroupFacets は、Connections の板の横（Settings がページを並べる位置）に置くグループの一覧である。
// グループを選ぶと接続の一覧がそのグループに絞られ、グループの上に落とした接続先とグループはそこへ移る。
// 狭い画面では、接続の一覧の中のシートからグループを選ぶ。
export function ConnectionGroupFacets({
  overview,
  scope,
  onScopeChange,
  drag,
  groupActions,
  onCreateGroup,
  folderNotices = [],
  onDeclareGroup,
}: {
  overview: Overview;
  scope: Scope;
  onScopeChange: (scope: Scope) => void;
  drag: ConnectionDrag;
  groupActions?: (name: string) => ActionMenuItem[];
  onCreateGroup?: () => void;
  // フォルダとグループの食い違い（folderNoticesOf）。Include 行の無いフォルダは onDeclareGroup でグループにする。
  folderNotices?: readonly Notice[];
  onDeclareGroup?: (name: string) => void;
}) {
  const t = useTranslate();
  const [width, setWidth] = useColumnWidth(connectionGroupsWidth);
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(new Set());
  const [openMenu, setOpenMenu] = useState<string | null>(null);
  // 隠したグループ（中に接続先の無い入れ物）は、選んだときだけ一覧に出し、「…」から隠すのをやめられるようにする。
  const [showHidden, setShowHidden] = useState(false);
  const decorated = useMemo(() => decorateHosts(overview), [overview]);
  const groupTree = useMemo(() => buildGroupTree(overview, decorated), [decorated, overview]);
  const ungrouped = decorated.filter((item) => item.group === "").length;
  // 入れ子のグループの開閉の印には、行の先頭に余白が要る。入れ子が無ければ余白を取らず、どの行も印から始める。
  const nested = overview.groups.some((group) => group.name.includes("/"));
  const gutter = nested ? <span aria-hidden="true" className="w-4 shrink-0" /> : null;
  const hiddenCount = countHidden(groupTree);

  function toggle(name: string) {
    setCollapsed((current) => {
      const next = new Set(current);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  }

  function renderGroup(node: GroupNode, depth = 0): ReactNode {
    const tucked = node.hidden && node.descendantCount === 0;
    if (tucked && !showHidden) {
      return <Fragment key={node.name}>{node.children.map((child) => renderGroup(child, depth))}</Fragment>;
    }
    const shut = collapsed.has(node.name);
    const active = scope.kind === "group" && scope.name === node.name;
    const menuKey = node.name;
    const items = groupActions?.(node.name) ?? [];
    return (
      <div key={node.name}>
        <div
          className={`group/row relative flex items-center rounded-[7px] ${dropTargetLook({ accepts: drag.accepts(node.name), over: drag.isOver(node.name) })}`}
          style={{ paddingInlineStart: `calc(${depth} * ${depthIndent})` }}
          onContextMenu={items.length === 0 ? undefined : (event) => {
            event.preventDefault();
            setOpenMenu(menuKey);
          }}
          {...drag.dropHandlers(node.name)}
        >
          {/* 開閉の印は、行の先頭の余白にボタンの上から重ねる。行全体を1つの押せる所のままにするため。
              見た目の順（印 → 名前）と Tab の順をそろえるため、名前のボタンより前に置く。 */}
          {node.children.length === 0 ? null : (
            <button
              type="button"
              aria-label={t(shut ? "tree.expand" : "tree.collapse", { name: node.name })}
              aria-expanded={!shut}
              onClick={() => toggle(node.name)}
              style={{ insetInlineStart: `calc(${depth} * ${depthIndent} + 0.375rem)` }}
              className="absolute top-1/2 flex size-5 -translate-y-1/2 items-center justify-center rounded text-ink-faint hover:text-ink"
            >
              <DisclosureChevron expanded={!shut} className="size-3.5" />
            </button>
          )}
          <button
            type="button"
            aria-label={node.name}
            title={node.name}
            aria-pressed={active}
            draggable={!drag.movesDisabled}
            onDragStart={(event) => drag.startDrag(event, { kind: "group", name: node.name })}
            onDragEnd={drag.endDrag}
            onClick={() => onScopeChange({ kind: "group", name: node.name })}
            className={`${facetLook(active)} ${drag.movesDisabled ? "" : draggableCursor} ${tucked ? "opacity-60" : ""}`}
          >
            {gutter}
            <span className={`flex shrink-0 ${active && node.colour === "" ? "text-accent" : ""}`} style={node.colour === "" ? undefined : { color: node.colour }}>
              <Icon name="groups" className="size-[15px]" />
            </span>
            <span className="min-w-0 flex-1 truncate">{node.label}</span>
            {tucked ? <span className="shrink-0 text-[11px] text-ink-faint">{t("tree.hiddenMark")}</span> : null}
            <span className={`shrink-0 font-mono text-[11.5px] font-normal text-ink-faint ${items.length === 0 ? "" : hideOnHoverAtPcSize} ${openMenu === menuKey ? "invisible" : ""}`}>
              {node.descendantCount}
            </span>
          </button>
          {items.length === 0 ? null : (
            <div className={`absolute right-1 top-1/2 -translate-y-1/2 ${openMenu === menuKey ? "opacity-100" : revealOnHoverAtPcSize}`}>
              <ActionMenu
                label={t("tree.groupActions", { name: node.name })}
                items={items}
                open={openMenu === menuKey}
                onOpenChange={(open) => setOpenMenu(open ? menuKey : null)}
                triggerClassName="flex size-6 items-center justify-center rounded text-ink-muted hover:bg-hover hover:text-ink"
                iconClassName="size-3.5"
              />
            </div>
          )}
        </div>
        {shut ? null : node.children.map((child) => renderGroup(child, depth + 1))}
      </div>
    );
  }

  const allActive = scope.kind === "all";
  const ungroupedActive = scope.kind === "ungrouped";
  return (
    <nav
      aria-label={t("tree.byGroups")}
      style={{ width: `${width}px` }}
      className="relative hidden shrink-0 flex-col py-4 pl-2.5 pr-3 lg:flex"
    >
      <div className="flex items-center justify-between gap-1 px-2 pb-2">
        <h2 className="text-xs font-medium text-ink-faint">{t("tree.byGroups")}</h2>
        {onCreateGroup === undefined ? null : (
          <button
            type="button"
            aria-label={t("tree.newGroup")}
            title={t("tree.newGroup")}
            onClick={onCreateGroup}
            className="-my-1 flex size-6 items-center justify-center rounded text-ink-faint hover:bg-hover hover:text-ink"
          >
            <Icon name="plus" className="size-3.5" />
          </button>
        )}
      </div>
      {onDeclareGroup === undefined ? null : (
        <div className="px-0.5 pb-2">
          <GroupFolderNotices notices={folderNotices} onDeclare={onDeclareGroup} />
        </div>
      )}
      <div data-connection-facets className="flex min-h-0 min-w-0 flex-1 flex-col gap-0.5 overflow-y-auto overflow-x-hidden">
        <button type="button" aria-label={t("tree.allConnections")} aria-pressed={allActive} onClick={() => onScopeChange({ kind: "all" })} className={facetLook(allActive)}>
          {gutter}
          <Icon name="groups" className={`size-[15px] shrink-0 ${allActive ? "text-accent" : ""}`} />
          <span className="min-w-0 flex-1 truncate">{t("tree.allConnections")}</span>
          <span className="shrink-0 font-mono text-[11.5px] font-normal text-ink-faint">{decorated.length}</span>
        </button>
        {groupTree.map((node) => renderGroup(node))}
        <div {...drag.dropHandlers("")} className={`rounded-[7px] ${dropTargetLook({ accepts: drag.accepts(""), over: drag.isOver("") })}`}>
          <button type="button" aria-label={t("tree.ungrouped")} aria-pressed={ungroupedActive} onClick={() => onScopeChange({ kind: "ungrouped" })} className={facetLook(ungroupedActive)}>
            {gutter}
            <Icon name="groups" className={`size-[15px] shrink-0 ${ungroupedActive ? "text-accent" : ""}`} />
            <span className="min-w-0 flex-1 truncate">{t("tree.ungrouped")}</span>
            <span className="shrink-0 font-mono text-[11.5px] font-normal text-ink-faint">{ungrouped}</span>
          </button>
        </div>
        {hiddenCount === 0 ? null : (
          <button
            type="button"
            aria-pressed={showHidden}
            onClick={() => setShowHidden((current) => !current)}
            className="mt-2 self-start rounded px-2 py-1 text-xs text-ink-faint hover:bg-hover hover:text-ink"
          >
            {t(showHidden ? "tree.hideHiddenGroups" : "tree.showHiddenGroups", { count: hiddenCount })}
          </button>
        )}
      </div>
      <ColumnResizeHandle
        label={t("conn.resizeGroups")}
        width={width}
        minimum={connectionGroupsWidth.minimum}
        maximum={connectionGroupsWidth.maximum}
        onWidthChange={setWidth}
        className="hidden lg:flex"
      />
    </nav>
  );
}

// countHidden は、一覧から外している隠したグループ（中に接続先が無いもの）を数える。
function countHidden(nodes: readonly GroupNode[]): number {
  return nodes.reduce((sum, node) => sum + (node.hidden && node.descendantCount === 0 ? 1 : 0) + countHidden(node.children), 0);
}
