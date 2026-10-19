import { Fragment, useEffect, useId, useMemo, useState, type CSSProperties, type DragEvent, type MouseEvent, type PointerEvent, type ReactNode } from "react";
import { DisclosureChevron } from "../ui/DisclosureChevron";
import type { HostEntry, HostMetadata, Overview } from "../api/config";
import { useTranslate } from "../i18n/context";
import { nearestDeclaredParent } from "./connectionBrowser";
import { hostMatchesQuery, normalizeHostQuery } from "./hostSearch";
import { ColumnResizeHandle } from "../ui/ColumnResizeHandle";
import { useStoredColumnWidth, type StoredColumnWidth } from "../ui/useStoredColumnWidth";
import { localStorageKeys } from "../ui/browserStorageKeys";
import { control } from "../ui/form";
import { Icon } from "../ui/icons";
import { useLongPress } from "../ui/useLongPress";
import { ActionMenu, type ActionMenuItem } from "../ui/ActionMenu";
import { duplicateAliasesOf, hostMetadataByIdentity, identityKey } from "./connectionBrowser";
import { canDrop, dragMimeType, type DragPayload } from "./dragdrop";
import { applySelectionGesture, emptyMultiSelection, keepVisible, type MultiSelection } from "./multiSelection";
import { GroupSheet } from "./GroupSheet";
import { ConnectionRow, revealOnHover } from "./ConnectionRow";
import { listGroups, ungroupedCount } from "./groupListing";

export type HostSelection = { path: string; alias: string };

type DecoratedHost = {
  host: HostEntry;
  key: string;
  projectionIdentity: string;
  projectionOrder: number;
  group: string;
  tags: string[];
  colour: string;
  os: HostMetadata["os"];
  order: number;
  sourceOrder: number;
  duplicateAlias: boolean;
};

type GroupNode = {
  name: string;
  label: string;
  hidden: boolean;
  order: number;
  sourceOrder: number;
  descendantCount: number;
  children: GroupNode[];
};

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
  onDrop: (payload: DragPayload, target: string) => void;
  movesDisabled?: boolean;
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

function belongsToGroup(item: DecoratedHost, group: string): boolean {
  return item.group === group || item.group.startsWith(`${group}/`);
}

function hostBlockIdentity(host: HostEntry): string {
  return JSON.stringify([
    host.file.absolute,
    host.line,
    host.identity.path,
    host.identity.alias,
  ]);
}

// Fits a nested group name next to its "…" button without stealing the list's room.
const connectionGroupsWidth: StoredColumnWidth = { key: localStorageKeys.connectionGroupsWidth, fallback: 184, minimum: 112, maximum: 400 };


export function ConnectionTree({
  overview,
  selected,
  onSelect,
  onDrop,
  movesDisabled = false,
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
  topBar = null,
  bottomBar = null,
}: ConnectionTreeProps) {
  const t = useTranslate();
  const [groupsWidth, setGroupsWidth] = useStoredColumnWidth(connectionGroupsWidth);
  const descriptionIdPrefix = useId();
  const [ownScope, setOwnScope] = useState<Scope>({ kind: "all" });
  const scope = controlledScope ?? ownScope;
  const setScope = onScopeChange ?? setOwnScope;
  const [query, setQuery] = useState("");
  const [sortOrder, setSortOrder] = useState<SortOrder>("name");
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(new Set());
  const [dragging, setDragging] = useState<DragPayload | null>(null);
  const [openMenu, setOpenMenu] = useState<string | null>(null);
  const [groupSheetOpen, setGroupSheetOpen] = useState(false);
  const longPress = useLongPress();
  const multiSelectable = onCheckedChange !== undefined;

  const groupNames = useMemo(() => overview.groups.map((group) => group.name), [overview.groups]);
  const decorated = useMemo<DecoratedHost[]>(() => {
    const metadata = hostMetadataByIdentity(overview.metadata.hosts);
    const duplicates = duplicateAliasesOf(overview.hosts);
    const projectionOccurrences = new Map<string, number>();
    return overview.hosts.flatMap((host, sourceOrder) => {
      if (host.identity.alias === "") return [];
      const blockIdentity = hostBlockIdentity(host);
      const projectionOccurrence = projectionOccurrences.get(blockIdentity) ?? 0;
      projectionOccurrences.set(blockIdentity, projectionOccurrence + 1);
      const display = metadata.get(identityKey(host.identity));
      return [{
        host,
        key: identityKey(host.identity),
        // Include traversal may project the same physical Host block more than once.
        // Retain that occurrence in the render identity so display sorting/filtering
        // cannot collapse React siblings back onto the file-and-line identity.
        projectionIdentity: `${blockIdentity}\u0000${projectionOccurrence}`,
        projectionOrder: sourceOrder,
        group: host.group ?? "",
        tags: display?.tags ?? [],
        colour: display?.colour ?? "",
        os: display?.os || display?.detectedOS || "",
        order: display?.order ?? 0,
        sourceOrder,
        duplicateAlias: duplicates.has(host.identity.alias),
      }];
    });
  }, [overview.hosts, overview.metadata.hosts]);

  const groupTree = useMemo(() => {
    const declared = new Set(overview.groups.map((group) => group.name));
    const metadata = new Map((overview.metadata.groups ?? []).map((group) => [group.name, group]));
    const nodes = new Map<string, GroupNode>();
    overview.groups.forEach((group, sourceOrder) => {
      const display = metadata.get(group.name);
      nodes.set(group.name, {
        name: group.name,
        label: group.name.slice(group.name.lastIndexOf("/") + 1),
        hidden: display?.hidden === true,
        order: display?.order ?? group.order ?? 0,
        sourceOrder,
        descendantCount: decorated.filter((item) => belongsToGroup(item, group.name)).length,
        children: [],
      });
    });
    const roots: GroupNode[] = [];
    for (const node of nodes.values()) {
      const parentName = nearestDeclaredParent(node.name, declared);
      const parent = parentName === "" ? undefined : nodes.get(parentName);
      if (parent === undefined) roots.push(node);
      else parent.children.push(node);
    }
    const sort = (nodesToSort: GroupNode[]) => {
      nodesToSort.sort((left, right) => left.order - right.order || left.sourceOrder - right.sourceOrder);
      nodesToSort.forEach((node) => sort(node.children));
    };
    sort(roots);
    return roots;
  }, [decorated, overview.groups, overview.metadata.groups]);

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
      if (event.key !== "Escape" || event.defaultPrevented) return;
      onCheckedChange(emptyMultiSelection);
      onSelectionModeChange?.(false);
    };
    window.addEventListener("keydown", clearOnEscape);
    return () => window.removeEventListener("keydown", clearOnEscape);
  }, [checked.keys.size, onCheckedChange, onSelectionModeChange]);

  const scopeLabel = scope.kind === "all" ? t("tree.allConnections") : scope.kind === "ungrouped" ? t("tree.ungrouped") : scope.name;
  const openedKey = selected === null ? null : identityKey(selected);

  function startDrag(event: DragEvent, payload: DragPayload) {
    if (movesDisabled) return;
    event.dataTransfer.setData(dragMimeType, JSON.stringify(payload));
    event.dataTransfer.effectAllowed = "move";
    setDragging(payload);
  }

  function dragPayloadFor(item: DecoratedHost): DragPayload {
    if (checked.keys.size >= 2 && checked.keys.has(item.key)) {
      const items = decorated
        .filter((candidate) => checked.keys.has(candidate.key))
        .map((candidate) => ({ path: candidate.host.identity.path, alias: candidate.host.identity.alias, group: candidate.group }));
      return { kind: "connections", items };
    }
    return { kind: "connection", path: item.host.identity.path, alias: item.host.identity.alias, group: item.group };
  }

  function accepts(target: string): boolean {
    return !movesDisabled && dragging !== null && canDrop(dragging, target, groupNames);
  }

  function dropHandlers(target: string) {
    return {
      onDragOver: (event: DragEvent) => {
        if (!accepts(target)) return;
        event.preventDefault();
        event.stopPropagation();
        event.dataTransfer.dropEffect = "move";
      },
      onDrop: (event: DragEvent) => {
        if (dragging === null || !accepts(target)) return;
        event.preventDefault();
        event.stopPropagation();
        onDrop(dragging, target);
        setDragging(null);
      },
    };
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

  function facetClass(active: boolean): string {
    return `flex min-h-10 w-auto items-center gap-2 rounded-r-md border-l-2 px-2 py-1.5 text-left text-xs transition-colors md:min-h-0 md:w-full ${
      active ? "border-accent bg-select-fill font-medium text-accent" : "border-transparent text-ink-muted hover:bg-hover hover:text-ink"
    }`;
  }

  function renderGroupFacet(node: GroupNode, depth = 0): ReactNode {
    if (node.hidden && node.descendantCount === 0) {
      return <Fragment key={node.name}>{node.children.map((child) => renderGroupFacet(child, depth))}</Fragment>;
    }
    const shut = collapsed.has(node.name);
    const active = scope.kind === "group" && scope.name === node.name;
    const padding = { "--sshc-facet-depth": depth } as CSSProperties;
    const menuKey = `group:${node.name}`;
    const items = groupActions?.(node.name) ?? [];
    return (
      <div key={node.name} className="min-w-max md:min-w-0">
        <div
          className={`group/row flex items-center rounded-md ${accepts(node.name) ? "bg-select-fill outline outline-1 outline-accent" : ""}`}
          style={padding}
          onContextMenu={items.length === 0 ? undefined : (event) => {
            event.preventDefault();
            setOpenMenu(menuKey);
          }}
          {...dropHandlers(node.name)}
        >
          {node.children.length === 0 ? (
            <span aria-hidden="true" className="w-5 shrink-0 md:ms-[calc(var(--sshc-facet-depth)*0.65rem)]" />
          ) : (
            <button
              type="button"
              aria-label={t(shut ? "tree.expand" : "tree.collapse", { name: node.name })}
              aria-expanded={!shut}
              onClick={() => setCollapsed((current) => {
                const next = new Set(current);
                if (next.has(node.name)) next.delete(node.name);
                else next.add(node.name);
                return next;
              })}
              className="ms-0 flex size-8 shrink-0 items-center justify-center rounded text-ink-faint hover:text-ink md:ms-[calc(var(--sshc-facet-depth)*0.65rem)] md:size-5"
            >
              <DisclosureChevron expanded={!shut} className="size-3.5" />
            </button>
          )}
          <button
            type="button"
            aria-label={node.name}
            title={node.name}
            aria-pressed={active}
            draggable={!movesDisabled}
            onDragStart={(event) => startDrag(event, { kind: "group", name: node.name })}
            onDragEnd={() => setDragging(null)}
            onClick={() => setScope({ kind: "group", name: node.name })}
            className={`${facetClass(active)} min-w-0 flex-1 px-1.5`}
          >
            <Icon name="groups" className="size-4" />
            <span className="min-w-0 flex-1 truncate">{node.label}</span>
            <span className={`shrink-0 text-[0.68rem] text-ink-faint ${items.length === 0 ? "" : "group-hover/row:hidden group-focus-within/row:hidden"} ${openMenu === menuKey ? "hidden" : ""}`}>{node.descendantCount}</span>
          </button>
          {items.length === 0 ? null : (
            <div className={`-ms-7 ${openMenu === menuKey ? "opacity-100" : revealOnHover}`}>
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
        {shut ? null : node.children.map((child) => renderGroupFacet(child, depth + 1))}
      </div>
    );
  }

  function renderFacets() {
    const allActive = scope.kind === "all";
    const ungroupedActive = scope.kind === "ungrouped";
    const ungrouped = decorated.filter((item) => item.group === "").length;
    return (
      <>
        <button type="button" aria-label={t("tree.allConnections")} aria-pressed={allActive} onClick={() => setScope({ kind: "all" })} className={facetClass(allActive)}>
          <Icon name="groups" className="size-4" />
          <span className="min-w-0 flex-1 truncate">{t("tree.allConnections")}</span>
          <span className="text-[0.68rem] text-ink-faint">{decorated.length}</span>
        </button>
        {groupTree.map((node) => renderGroupFacet(node))}
        <div {...dropHandlers("")} className={accepts("") ? "rounded-md outline outline-1 outline-accent" : ""}>
          <button type="button" aria-label={t("tree.ungrouped")} aria-pressed={ungroupedActive} onClick={() => setScope({ kind: "ungrouped" })} className={`${facetClass(ungroupedActive)} min-w-max md:min-w-0`}>
            <Icon name="groups" className="size-4" />
            <span className="min-w-0 flex-1 truncate">{t("tree.ungrouped")}</span>
            <span className="text-[0.68rem] text-ink-faint">{ungrouped}</span>
          </button>
        </div>
      </>
    );
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
        draggable={!movesDisabled && !selectionMode}
        menuItems={selectionMode ? [] : connectionActions?.(host) ?? []}
        menuOpen={openMenu === menuKey}
        onMenuOpenChange={(open) => setOpenMenu(open ? menuKey : null)}
        onClick={(event) => clickRow(event, item)}
        onPointerDown={(event) => beginLongPress(event, item)}
        onPointerEnd={longPress.cancel}
        onDragStart={(event) => startDrag(event, dragPayloadFor(item))}
        onDragEnd={() => setDragging(null)}
      />
    );
  }

  function renderResultSection(section: ResultSection) {
    const name = section.name === "" ? t("tree.ungrouped") : section.name;
    return (
      <section key={section.name || "ungrouped"} data-connection-group={section.name || "ungrouped"} aria-label={t("tree.groupSection", { name, count: section.items.length })}>
        <header className="sticky top-0 z-[1] flex min-h-8 items-center gap-2 border-y border-line bg-tree px-3 py-2 text-[0.68rem] text-ink-muted">
          <Icon name="groups" className="size-3.5 shrink-0 text-ink-faint" />
          <h3 className="min-w-0 flex-1 truncate font-medium" title={name}>{name}</h3>
          <span className="shrink-0 font-mono tabular-nums text-ink-faint">{section.items.length}</span>
        </header>
        {section.items.length === 0 ? null : <ul>{section.items.map(renderItem)}</ul>}
      </section>
    );
  }

  return (
    <nav
      aria-label={t("tree.navLabel")}
      style={{ "--groups-width": `${groupsWidth}px` } as CSSProperties}
      className="grid h-full min-h-0 grid-cols-1 grid-rows-[minmax(0,1fr)] lg:grid-cols-[var(--groups-width)_minmax(0,1fr)] md:grid-rows-1"
    >
      <aside className="relative hidden min-h-0 flex-col border-r border-line bg-tree lg:flex">
        <div className="flex shrink-0 items-center justify-between gap-1 px-3 pb-1 pt-2">
          <p className="text-[0.68rem] font-semibold uppercase tracking-[0.12em] text-ink-faint">
            {t("tree.byGroups")}
          </p>
          {onCreateGroup === undefined ? null : (
            <button
              type="button"
              aria-label={t("tree.newGroup")}
              title={t("tree.newGroup")}
              onClick={onCreateGroup}
              className="flex size-6 items-center justify-center rounded text-ink-muted hover:bg-hover hover:text-ink"
            >
              <Icon name="plus" className="size-4" />
            </button>
          )}
        </div>
        <div data-connection-facets className="flex min-h-0 min-w-0 flex-1 flex-col gap-1 overflow-x-hidden overflow-y-auto px-3 pb-3">
          {renderFacets()}
        </div>
        <ColumnResizeHandle
          label={t("conn.resizeGroups")}
          width={groupsWidth}
          minimum={connectionGroupsWidth.minimum}
          maximum={connectionGroupsWidth.maximum}
          onWidthChange={setGroupsWidth}
          className="hidden lg:flex"
        />
      </aside>

      <section className="flex min-h-0 flex-col bg-card" aria-label={t("tree.resultsLabel")}>
        <header className="shrink-0 border-b border-line bg-card p-3">
          <button
            type="button"
            onClick={() => setGroupSheetOpen(true)}
            className={`${control} mb-2 flex min-h-10 items-center gap-2 py-2 text-left lg:hidden`}
          >
            <Icon name="groups" className="size-4 text-ink-faint" />
            <span className="min-w-0 flex-1 truncate">{t("tree.groupButton", { name: scopeLabel })}</span>
            <DisclosureChevron expanded={false} className="size-3.5 rotate-90" />
          </button>
          <div>
            <label className="relative block min-w-0" htmlFor="connection-filter">
              <span className="sr-only">{t("tree.filter")}</span>
              <Icon name="search" className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-ink-faint" />
              <input id="connection-filter" type="search" value={query} onChange={(event) => setQuery(event.currentTarget.value)} placeholder={t("tree.filterPlaceholderExpanded")} className={`${control} min-h-10 rounded-lg bg-control py-2 pl-9 pr-3 md:min-h-0`} />
            </label>
          </div>
          <div className="mt-2 flex items-center justify-between gap-3 text-xs text-ink-faint">
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
                <select value={sortOrder} onChange={(event) => setSortOrder(event.currentTarget.value as SortOrder)} className="rounded border-0 bg-transparent py-0.5 text-xs text-ink-muted">
                  <option value="configured">{t("tree.sortConfigured")}</option>
                  <option value="name">{t("tree.sortName")}</option>
                </select>
              </label>
            </span>
          </div>
          {!movesDisabled ? <p className="sr-only">{t("tree.dragGroupHint")}</p> : null}
        </header>
        {topBar}
        {visible.length === 0 ? <p role="status" className="p-5 text-sm text-ink-muted">{t("tree.noMatch")}</p> : <div data-connection-results className="min-h-0 flex-1 overflow-y-auto">{resultSections.map(renderResultSection)}</div>}
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
          onClose={() => setGroupSheetOpen(false)}
        />
      ) : null}
    </nav>
  );
}
