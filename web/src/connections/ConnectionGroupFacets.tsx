import { Fragment, useMemo, useState, type ReactNode } from "react";
import type { Overview } from "../api/config";
import { useTranslate } from "../i18n/context";
import { ActionMenu, revealOnHover, type ActionMenuItem } from "../ui/ActionMenu";
import { ColumnResizeHandle } from "../ui/ColumnResizeHandle";
import { DisclosureChevron } from "../ui/DisclosureChevron";
import { draggableCursor, dropTargetLook } from "../ui/dragAndDropLook";
import { Icon } from "../ui/icons";
import { localStorageKeys } from "../ui/browserStorageKeys";
import { useStoredColumnWidth, type StoredColumnWidth } from "../ui/useStoredColumnWidth";
import type { Scope } from "./ConnectionTree";
import { buildGroupTree, decorateHosts, type GroupNode } from "./connectionTreeIndex";
import type { ConnectionDrag } from "./useConnectionDrag";

// 220px は Settings のページの一覧の幅。この列は同じ位置に置く。
const connectionGroupsWidth: StoredColumnWidth = { key: localStorageKeys.connectionGroupsWidth, fallback: 220, minimum: 112, maximum: 400 };

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
}: {
  overview: Overview;
  scope: Scope;
  onScopeChange: (scope: Scope) => void;
  drag: ConnectionDrag;
  groupActions?: (name: string) => ActionMenuItem[];
  onCreateGroup?: () => void;
}) {
  const t = useTranslate();
  const [width, setWidth] = useStoredColumnWidth(connectionGroupsWidth);
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(new Set());
  const [openMenu, setOpenMenu] = useState<string | null>(null);
  const decorated = useMemo(() => decorateHosts(overview), [overview]);
  const groupTree = useMemo(() => buildGroupTree(overview, decorated), [decorated, overview]);
  const ungrouped = decorated.filter((item) => item.group === "").length;
  // 入れ子のグループの開閉の印には、行の先頭に余白が要る。入れ子が無ければ余白を取らず、どの行も印から始める。
  const nested = overview.groups.some((group) => group.name.includes("/"));
  const gutter = nested ? <span aria-hidden="true" className="w-4 shrink-0" /> : null;

  function toggle(name: string) {
    setCollapsed((current) => {
      const next = new Set(current);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  }

  function renderGroup(node: GroupNode, depth = 0): ReactNode {
    if (node.hidden && node.descendantCount === 0) {
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
          <button
            type="button"
            aria-label={node.name}
            title={node.name}
            aria-pressed={active}
            draggable={!drag.movesDisabled}
            onDragStart={(event) => drag.startDrag(event, { kind: "group", name: node.name })}
            onDragEnd={drag.endDrag}
            onClick={() => onScopeChange({ kind: "group", name: node.name })}
            className={`${facetLook(active)} ${drag.movesDisabled ? "" : draggableCursor}`}
          >
            {gutter}
            <span className={`flex shrink-0 ${active && node.colour === "" ? "text-accent" : ""}`} style={node.colour === "" ? undefined : { color: node.colour }}>
              <Icon name="groups" className="size-[15px]" />
            </span>
            <span className="min-w-0 flex-1 truncate">{node.label}</span>
            <span className={`shrink-0 font-mono text-[11.5px] font-normal text-ink-faint ${items.length === 0 ? "" : "group-hover/row:invisible group-focus-within/row:invisible"} ${openMenu === menuKey ? "invisible" : ""}`}>
              {node.descendantCount}
            </span>
          </button>
          {/* 開閉の印は、行の先頭の余白にボタンの上から重ねる。行全体を1つの押せる所のままにするため。 */}
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
          {items.length === 0 ? null : (
            <div className={`absolute right-1 top-1/2 -translate-y-1/2 ${openMenu === menuKey ? "opacity-100" : revealOnHover}`}>
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
