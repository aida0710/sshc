import { useEffect, useId, useMemo, useState, type MouseEvent, type PointerEvent, type ReactNode } from "react";
import { DisclosureChevron } from "../ui/DisclosureChevron";
import type { HostEntry, Notice, Overview } from "../api/config";
import { useTranslate } from "../i18n/context";
import { hostMatchesQuery, normalizeHostQuery } from "./hostSearch";
import { control } from "../ui/form";
import { Icon } from "../ui/icons";
import { useLongPress } from "../ui/useLongPress";
import type { ActionMenuItem } from "../ui/ActionMenu";
import { identityKey } from "./connectionBrowser";
import type { DragPayload } from "./dragdrop";
import { applySelectionGesture, emptyMultiSelection, keepVisible, type MultiSelection } from "./multiSelection";
import { GroupSheet } from "./GroupSheet";
import { ConnectionRow } from "./ConnectionRow";
import { listGroups, ungroupedCount } from "./groupListing";
import { isImeComposing } from "../ui/imeComposition";
import { belongsToGroup, buildGroupTree, decorateHosts, type DecoratedHost, type GroupNode } from "./connectionTreeIndex";
import type { ConnectionDrag } from "./useConnectionDrag";

export type HostSelection = { path: string; alias: string };

type ResultSection = {
  name: string;
  items: DecoratedHost[];
};

export type Scope =
  | { kind: "all" }
  | { kind: "group"; name: string }
  | { kind: "ungrouped" };

export type ConnectionTreeProps = {
  overview: Overview;
  selected: HostSelection | null;
  onSelect: (host: HostEntry) => void;
  // drag は、板の横のグループの一覧と分け合う。行はそのグループの上に落とす。
  drag: ConnectionDrag;
  // 以下は複数選択とグループの操作。渡さなければ今までどおり1件ずつ選ぶ一覧になる。
  scope?: Scope;
  onScopeChange?: (scope: Scope) => void;
  checked?: MultiSelection;
  onCheckedChange?: (next: MultiSelection) => void;
  // 編集中の下書きがあるあいだは、Shift と Ctrl で選ばせない（選ぶとエディタが入れ替わるため）。
  multiSelectDisabled?: boolean;
  compact?: boolean;
  selectionMode?: boolean;
  onSelectionModeChange?: (on: boolean) => void;
  groupActions?: (name: string) => ActionMenuItem[];
  connectionActions?: (host: HostEntry) => ActionMenuItem[];
  onCreateGroup?: () => void;
  // スマホのグループのシートに出す、フォルダとグループの食い違い（folderNoticesOf）。
  folderNotices?: readonly Notice[];
  onDeclareGroup?: (name: string) => void;
  topBar?: ReactNode;
  bottomBar?: ReactNode;
};

type SortOrder = "configured" | "name";

function labelFor(host: HostEntry): string {
  return host.identity.alias === "" ? `Host ${host.patterns.join(" ")}` : host.identity.alias;
}

function hostMatches(item: DecoratedHost, normalizedQuery: string): boolean {
  const host = item.host;
  return hostMatchesQuery({
    alias: host.identity.alias, group: item.group, patterns: host.patterns, tags: item.tags,
    ...(host.hostName === undefined ? {} : { hostName: host.hostName }),
    ...(host.user === undefined ? {} : { user: host.user }),
  }, normalizedQuery);
}

export function ConnectionTree({
  overview,
  selected,
  onSelect,
  drag,
  scope: controlledScope,
  onScopeChange,
  checked = emptyMultiSelection,
  onCheckedChange,
  multiSelectDisabled = false,
  compact = false,
  selectionMode = false,
  onSelectionModeChange,
  groupActions,
  connectionActions,
  onCreateGroup,
  folderNotices = [],
  onDeclareGroup,
  topBar = null,
  bottomBar = null,
}: ConnectionTreeProps) {
  const t = useTranslate();
  const descriptionIdPrefix = useId();
  const [ownScope, setOwnScope] = useState<Scope>({ kind: "all" });
  const scope = controlledScope ?? ownScope;
  const setScope = onScopeChange ?? setOwnScope;
  const [query, setQuery] = useState("");
  const [sortOrder, setSortOrder] = useState<SortOrder>("name");
  const [openMenu, setOpenMenu] = useState<string | null>(null);
  const [groupSheetOpen, setGroupSheetOpen] = useState(false);
  const longPress = useLongPress();
  const multiSelectable = onCheckedChange !== undefined;

  const decorated = useMemo(() => decorateHosts(overview), [overview]);
  const groupTree = useMemo(() => buildGroupTree(overview, decorated), [decorated, overview]);

  const orderedGroups = useMemo(() => {
    const ordered: GroupNode[] = [];
    const visit = (node: GroupNode) => {
      if (!(node.hidden && node.descendantCount === 0)) ordered.push(node);
      node.children.forEach(visit);
    };
    groupTree.forEach(visit);
    return ordered;
  }, [groupTree]);

  const visible = useMemo(() => {
    const normalizedQuery = normalizeHostQuery(query);
    const scoped = decorated.filter((item) => {
      if (!hostMatches(item, normalizedQuery)) return false;
      if (scope.kind === "all") return true;
      if (scope.kind === "ungrouped") return item.group === "";
      return belongsToGroup(item, scope.name);
    });
    return scoped.sort((left, right) => {
      if (sortOrder === "name") return labelFor(left.host).localeCompare(labelFor(right.host), "ja");
      return left.order - right.order || left.sourceOrder - right.sourceOrder;
    });
  }, [decorated, query, scope, sortOrder]);

  const resultSections = useMemo<ResultSection[]>(() => {
    const exact = new Map<string, DecoratedHost[]>();
    for (const item of visible) {
      const items = exact.get(item.group) ?? [];
      items.push(item);
      exact.set(item.group, items);
    }
    if (scope.kind === "ungrouped") {
      return [{ name: "", items: exact.get("") ?? [] }];
    }

    const declared = new Set(orderedGroups.map((group) => group.name));
    const sections = orderedGroups
      .filter((group) => scope.kind === "all" || group.name === scope.name || group.name.startsWith(`${scope.name}/`))
      .filter((group) => visible.some((item) => belongsToGroup(item, group.name)))
      .map((group) => ({ name: group.name, items: exact.get(group.name) ?? [] }));
    const undeclared = [...exact.keys()]
      .filter((name) => name !== "" && !declared.has(name))
      .sort((left, right) => left.localeCompare(right, "ja"));
    sections.push(...undeclared.map((name) => ({ name, items: exact.get(name) ?? [] })));
    if (scope.kind === "all" && exact.has("")) {
      sections.push({ name: "", items: exact.get("") ?? [] });
    }
    return sections;
  }, [orderedGroups, scope, visible]);

  // 範囲選択は、見出しで区切られた画面の上から下への順で数える。
  const visibleOrder = useMemo(() => resultSections.flatMap((section) => section.items.map((item) => item.key)), [resultSections]);

  useEffect(() => {
    if (onCheckedChange === undefined) return;
    const pruned = keepVisible(checked, new Set(visibleOrder));
    if (pruned !== checked) onCheckedChange(pruned);
  }, [checked, onCheckedChange, visibleOrder]);

  useEffect(() => {
    if (onCheckedChange === undefined || checked.keys.size === 0) return;
    const clearOnEscape = (event: KeyboardEvent) => {
      if (isImeComposing(event) || event.key !== "Escape" || event.defaultPrevented) return;
      onCheckedChange(emptyMultiSelection);
      onSelectionModeChange?.(false);
    };
    window.addEventListener("keydown", clearOnEscape);
    return () => window.removeEventListener("keydown", clearOnEscape);
  }, [checked.keys.size, onCheckedChange, onSelectionModeChange]);

  const scopeLabel = scope.kind === "all" ? t("tree.allConnections") : scope.kind === "ungrouped" ? t("tree.ungrouped") : scope.name;
  const openedKey = selected === null ? null : identityKey(selected);

  function dragPayloadFor(item: DecoratedHost): DragPayload {
    if (checked.keys.size >= 2 && checked.keys.has(item.key)) {
      const items = decorated
        .filter((candidate) => checked.keys.has(candidate.key))
        .map((candidate) => ({ path: candidate.host.identity.path, alias: candidate.host.identity.alias, group: candidate.group }));
      return { kind: "connections", items };
    }
    return { kind: "connection", path: item.host.identity.path, alias: item.host.identity.alias, group: item.group };
  }

  function toggleChecked(item: DecoratedHost) {
    if (onCheckedChange === undefined) return;
    onCheckedChange(applySelectionGesture({ current: checked, clicked: item.key, opened: null, visibleOrder, gesture: "toggle" }));
  }

  function clickRow(event: MouseEvent, item: DecoratedHost) {
    if (longPress.consumeClick()) return;
    if (selectionMode) {
      toggleChecked(item);
      return;
    }
    const gesture = event.shiftKey ? "range" : event.metaKey || event.ctrlKey ? "toggle" : null;
    if (gesture !== null && multiSelectable && !multiSelectDisabled) {
      onCheckedChange?.(applySelectionGesture({ current: checked, clicked: item.key, opened: openedKey, visibleOrder, gesture }));
      return;
    }
    // ふつうのクリックは1件だけを開き、それを次の Shift の基点にする。
    onCheckedChange?.({ keys: new Set(), anchor: item.key });
    onSelect(item.host);
  }

  function beginLongPress(event: PointerEvent, item: DecoratedHost) {
    if (!multiSelectable || onSelectionModeChange === undefined) return;
    longPress.start(event, () => {
      onSelectionModeChange(true);
      if (!checked.keys.has(item.key)) toggleChecked(item);
    });
  }

  function renderItem(item: DecoratedHost) {
    const host = item.host;
    const menuKey = `connection:${item.key}`;
    return (
      <ConnectionRow
        key={item.projectionIdentity}
        host={host}
        os={item.os}
        colour={item.colour}
        tags={item.tags}
        duplicateAlias={item.duplicateAlias}
        descriptionId={`${descriptionIdPrefix}-connection-${item.projectionOrder}`}
        opened={selected?.path === host.identity.path && selected.alias === host.identity.alias}
        chosen={checked.keys.has(item.key)}
        choosing={checked.keys.size > 0 || selectionMode}
        showCheckbox={selectionMode}
        draggable={!drag.movesDisabled && !selectionMode}
        menuItems={selectionMode ? [] : connectionActions?.(host) ?? []}
        menuOpen={openMenu === menuKey}
        onMenuOpenChange={(open) => setOpenMenu(open ? menuKey : null)}
        onClick={(event) => clickRow(event, item)}
        onPointerDown={(event) => beginLongPress(event, item)}
        onPointerEnd={longPress.cancel}
        onDragStart={(event) => drag.startDrag(event, dragPayloadFor(item))}
        onDragEnd={drag.endDrag}
      />
    );
  }

  function renderResultSection(section: ResultSection) {
    const name = section.name === "" ? t("tree.ungrouped") : section.name;
    const colour = (overview.metadata.groups ?? []).find((group) => group.name === section.name)?.colour ?? "";
    return (
      <section key={section.name || "ungrouped"} data-connection-group={section.name || "ungrouped"} aria-label={t("tree.groupSection", { name, count: section.items.length })}>
        <header className="sticky top-0 z-[1] flex items-center gap-1.5 bg-sidebar px-3.5 pb-1 pt-2.5 text-xs text-ink-faint">
          <span className="flex shrink-0" style={colour === "" ? undefined : { color: colour }}>
            <Icon name="groups" className="size-[13px]" />
          </span>
          <h3 className="min-w-0 flex-1 truncate font-normal" title={name}>{name}</h3>
        </header>
        {section.items.length === 0 ? null : <ul className="flex flex-col gap-px px-1.5">{section.items.map(renderItem)}</ul>}
      </section>
    );
  }

  return (
    <nav aria-label={t("tree.navLabel")} className="flex h-full min-h-0 flex-col">
      <section className="flex min-h-0 flex-1 flex-col" aria-label={t("tree.resultsLabel")}>
        <header className="flex shrink-0 flex-col gap-2 px-3.5 pb-2 pt-3.5">
          <button
            type="button"
            onClick={() => setGroupSheetOpen(true)}
            className={`${control} flex min-h-10 items-center gap-2 py-2 text-left lg:hidden`}
          >
            <Icon name="groups" className="size-4 text-ink-faint" />
            <span className="min-w-0 flex-1 truncate">{t("tree.groupButton", { name: scopeLabel })}</span>
            <DisclosureChevron expanded={false} className="size-3.5 rotate-90" />
          </button>
          <label className="relative block min-w-0" htmlFor="connection-filter">
            <span className="sr-only">{t("tree.filter")}</span>
            <Icon name="search" className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-ink-faint" />
            <input id="connection-filter" type="search" value={query} onChange={(event) => setQuery(event.currentTarget.value)} placeholder={t("hostSearch.placeholder")} className={`${control} min-h-10 pl-8 md:min-h-0`} />
          </label>
          <div className="flex items-center justify-between gap-3 whitespace-nowrap text-xs text-ink-faint">
            <span role="status">{t("tree.resultCount", { visible: visible.length, total: decorated.length })}</span>
            <span className="flex items-center gap-2">
              {compact && onSelectionModeChange !== undefined ? (
                <button
                  type="button"
                  aria-pressed={selectionMode}
                  onClick={() => {
                    if (selectionMode) onCheckedChange?.(emptyMultiSelection);
                    onSelectionModeChange(!selectionMode);
                  }}
                  className="min-h-9 rounded-md px-2 text-xs font-medium text-accent hover:bg-hover"
                >
                  {selectionMode ? t("tree.selectDone") : t("tree.select")}
                </button>
              ) : null}
              <label className="flex items-center gap-2">
                <span className="sr-only">{t("tree.sortLabel")}</span>
                <select value={sortOrder} onChange={(event) => setSortOrder(event.currentTarget.value as SortOrder)} className="rounded border-0 bg-transparent py-0.5 text-xs text-ink-faint">
                  <option value="configured">{t("tree.sortConfigured")}</option>
                  <option value="name">{t("tree.sortName")}</option>
                </select>
              </label>
            </span>
          </div>
          {!drag.movesDisabled ? <p className="sr-only">{t("tree.dragGroupHint")}</p> : null}
        </header>
        {topBar}
        {visible.length === 0 ? <p role="status" className="p-5 text-sm text-ink-muted">{t("tree.noMatch")}</p> : <div data-connection-results className={`min-h-0 flex-1 overflow-y-auto ${bottomBar === null ? "pb-[calc(0.5rem+var(--mobile-navigation-inset))] scroll-pb-(--mobile-navigation-inset)" : "pb-2"}`}>{resultSections.map(renderResultSection)}</div>}
        {bottomBar}
      </section>
      {groupSheetOpen ? (
        <GroupSheet
          groups={listGroups(overview)}
          total={decorated.length}
          ungrouped={ungroupedCount(overview)}
          active={scope}
          onChoose={setScope}
          onCreate={onCreateGroup === undefined ? null : () => {
            setGroupSheetOpen(false);
            onCreateGroup();
          }}
          groupActions={(name) => (groupActions?.(name) ?? []).map((item) => ({
            ...item,
            onSelect: () => {
              setGroupSheetOpen(false);
              item.onSelect();
            },
          }))}
          folderNotices={folderNotices}
          {...(onDeclareGroup === undefined ? {} : { onDeclare: (name: string) => {
            setGroupSheetOpen(false);
            onDeclareGroup(name);
          } })}
          onClose={() => setGroupSheetOpen(false)}
        />
      ) : null}
    </nav>
  );
}
