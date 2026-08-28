import { Fragment, useMemo, useState, type CSSProperties, type DragEvent, type ReactNode } from "react";
import type { HostEntry, Overview } from "../api/config";
import { useTranslate } from "../i18n/context";
import { control } from "../ui/form";
import { Icon } from "../ui/icons";
import { duplicateAliasesOf, identityKey } from "./connectionBrowser";
import { canDrop, dragMimeType, type DragPayload } from "./dragdrop";

export type HostSelection = { path: string; alias: string };

type DecoratedHost = {
  host: HostEntry;
  group: string;
  tags: string[];
  colour: string;
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

type ConnectionTreeProps = {
  overview: Overview;
  selected: HostSelection | null;
  onSelect: (host: HostEntry) => void;
  onDrop: (payload: DragPayload, target: string) => void;
  movesDisabled?: boolean;
};

type SortOrder = "configured" | "name";
type Scope =
  | { kind: "all" }
  | { kind: "group"; name: string }
  | { kind: "ungrouped" };

function labelFor(host: HostEntry): string {
  return host.identity.alias === "" ? `Host ${host.patterns.join(" ")}` : host.identity.alias;
}

function hostMatches(item: DecoratedHost, rawQuery: string): boolean {
  const query = rawQuery.trim().toLocaleLowerCase();
  if (query === "") return true;
  const host = item.host;
  return host.identity.alias.toLocaleLowerCase().includes(query) ||
    host.patterns.some((pattern) => pattern.toLocaleLowerCase().includes(query)) ||
    item.group.toLocaleLowerCase().includes(query) ||
    item.tags.some((tag) => tag.toLocaleLowerCase().includes(query)) ||
    (host.hostName ?? "").toLocaleLowerCase().includes(query) ||
    (host.user ?? "").toLocaleLowerCase().includes(query);
}

function nearestParent(name: string, declared: ReadonlySet<string>): string {
  let candidate = name;
  while (true) {
    const cut = candidate.lastIndexOf("/");
    if (cut < 0) return "";
    candidate = candidate.slice(0, cut);
    if (declared.has(candidate)) return candidate;
  }
}

function belongsToGroup(item: DecoratedHost, group: string): boolean {
  return item.group === group || item.group.startsWith(`${group}/`);
}

function targetFor(host: HostEntry): string {
  if ((host.hostName ?? "") === "") return host.file.path ?? host.file.absolute;
  const account = (host.user ?? "") === "" ? "" : `${host.user}@`;
  const port = (host.port ?? "") === "" || host.port === "22" ? "" : `:${host.port}`;
  return `${account}${host.hostName}${port}`;
}

export function ConnectionTree({
  overview,
  selected,
  onSelect,
  onDrop,
  movesDisabled = false,
}: ConnectionTreeProps) {
  const t = useTranslate();
  const [scope, setScope] = useState<Scope>({ kind: "all" });
  const [query, setQuery] = useState("");
  const [sortOrder, setSortOrder] = useState<SortOrder>("configured");
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(new Set());
  const [dragging, setDragging] = useState<DragPayload | null>(null);

  const groupNames = useMemo(() => overview.groups.map((group) => group.name), [overview.groups]);
  const decorated = useMemo<DecoratedHost[]>(() => {
    const metadata = new Map(
      (overview.metadata.hosts ?? []).map((entry) => [identityKey(entry.identity), entry]),
    );
    const duplicates = duplicateAliasesOf(overview.hosts);
    return overview.hosts.filter((host) => host.identity.alias !== "").map((host, sourceOrder) => {
      const display = metadata.get(identityKey(host.identity));
      return {
        host,
        group: host.group ?? "",
        tags: display?.tags ?? [],
        colour: display?.colour ?? "",
        order: display?.order ?? 0,
        sourceOrder,
        duplicateAlias: duplicates.has(host.identity.alias),
      };
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
      const parentName = nearestParent(node.name, declared);
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
    const scoped = decorated.filter((item) => {
      if (!hostMatches(item, query)) return false;
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

  const scopeValue = scope.kind === "all" ? "all" : scope.kind === "ungrouped" ? "ungrouped" : `group:${scope.name}`;

  function selectScope(value: string) {
    if (value === "all") setScope({ kind: "all" });
    else if (value === "ungrouped") setScope({ kind: "ungrouped" });
    else setScope({ kind: "group", name: value.slice("group:".length) });
    setDragging(null);
  }

  function startDrag(event: DragEvent, payload: DragPayload) {
    if (movesDisabled) return;
    event.dataTransfer.setData(dragMimeType, JSON.stringify(payload));
    event.dataTransfer.effectAllowed = "move";
    setDragging(payload);
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

  function facetClass(active: boolean): string {
    return `flex min-h-10 w-auto items-center gap-2 rounded-md px-2 py-1.5 text-left text-xs transition-colors md:min-h-0 md:w-full ${
      active ? "bg-select-fill text-ink" : "text-ink-muted hover:bg-surface hover:text-ink"
    }`;
  }

  function renderGroupFacet(node: GroupNode, depth = 0): ReactNode {
    if (node.hidden && node.descendantCount === 0) {
      return <Fragment key={node.name}>{node.children.map((child) => renderGroupFacet(child, depth))}</Fragment>;
    }
    const shut = collapsed.has(node.name);
    const active = scope.kind === "group" && scope.name === node.name;
    const padding = { "--sshc-facet-depth": depth } as CSSProperties;
    return (
      <div key={node.name} className="min-w-max md:min-w-0">
        <div
          className={`flex items-center rounded-md ${accepts(node.name) ? "bg-select-fill outline outline-1 outline-accent" : ""}`}
          style={padding}
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
              <span aria-hidden="true">{shut ? "▸" : "▾"}</span>
            </button>
          )}
          <button
            type="button"
            aria-label={node.name}
            aria-pressed={active}
            draggable={!movesDisabled}
            onDragStart={(event) => startDrag(event, { kind: "group", name: node.name })}
            onDragEnd={() => setDragging(null)}
            onClick={() => setScope({ kind: "group", name: node.name })}
            className={`${facetClass(active)} min-w-0 flex-1 px-1.5`}
          >
            <Icon name="groups" className="size-4" />
            <span className="min-w-0 flex-1 truncate">{node.label}</span>
            <span className="shrink-0 text-[0.68rem] text-ink-faint">{node.descendantCount}</span>
          </button>
        </div>
        {shut ? null : node.children.map((child) => renderGroupFacet(child, depth + 1))}
      </div>
    );
  }

  function renderFacets() {
    const allActive = scope.kind === "all";
    const ungroupedActive = scope.kind === "ungrouped";
    const ungroupedCount = decorated.filter((item) => item.group === "").length;
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
            <span className="text-[0.68rem] text-ink-faint">{ungroupedCount}</span>
          </button>
        </div>
      </>
    );
  }

  function renderItem(item: DecoratedHost) {
    const host = item.host;
    const active = selected?.path === host.identity.path && selected.alias === host.identity.alias;
    const descriptionId = `connection-${host.file.absolute}-${host.line}`;
    return (
      <li key={`${host.file.absolute}:${host.line}`} className="border-b border-hairline last:border-b-0">
        <button
          type="button"
          aria-label={host.identity.alias}
          aria-current={active ? "true" : undefined}
          aria-describedby={descriptionId}
          draggable={!movesDisabled}
          onClick={() => onSelect(host)}
          onDragStart={(event) => startDrag(event, { kind: "connection", path: host.identity.path, alias: host.identity.alias, group: item.group })}
          onDragEnd={() => setDragging(null)}
          className={`min-h-14 w-full px-4 py-2.5 text-left text-sm transition-colors ${active ? "bg-select-fill text-ink" : "text-ink-muted hover:bg-surface hover:text-ink"}`}
        >
          <span className="flex min-w-0 items-center gap-2">
            {item.colour === "" ? (
              <Icon name="terminal" className="size-4 text-ink-faint" />
            ) : (
              <span aria-hidden="true" className="size-2 shrink-0 rounded-full" style={{ backgroundColor: item.colour }} />
            )}
            <span className="min-w-0 flex-1">
              <span className="flex min-w-0 items-center gap-1.5">
                <span className={`truncate ${active ? "font-semibold" : "font-medium"}`}>{host.identity.alias}</span>
                {item.duplicateAlias ? <span aria-hidden="true" className="shrink-0 text-notice-ink">⧉</span> : null}
              </span>
              <span className="mt-0.5 block truncate font-mono text-[0.68rem] leading-4 text-ink-faint">{targetFor(host)}</span>
              {item.tags.length === 0 ? null : (
                <span aria-hidden="true" className="mt-1 flex flex-wrap gap-1">
                  {item.tags.map((tag) => <span key={tag} className="rounded bg-select-fill px-1 text-[0.65rem] text-ink-muted">{tag}</span>)}
                </span>
              )}
            </span>
          </span>
        </button>
        <span id={descriptionId} className="sr-only">
          {[item.duplicateAlias ? t("tree.duplicateAlias") : "", targetFor(host), host.file.path ?? host.file.absolute].filter(Boolean).join(", ")}
        </span>
      </li>
    );
  }

  function renderResultSection(section: ResultSection) {
    const name = section.name === "" ? t("tree.ungrouped") : section.name;
    return (
      <section key={section.name || "ungrouped"} data-connection-group={section.name || "ungrouped"} aria-label={t("tree.groupSection", { name, count: section.items.length })}>
        <header className="sticky top-0 z-[1] flex min-h-7 items-center gap-2 border-y border-hairline bg-tree px-3 py-1 text-[0.68rem] text-ink-muted">
          <Icon name="groups" className="size-3.5 shrink-0 text-ink-faint" />
          <h3 className="min-w-0 flex-1 truncate font-medium">{name}</h3>
          <span className="shrink-0 font-mono tabular-nums text-ink-faint">{section.items.length}</span>
        </header>
        {section.items.length === 0 ? null : <ul>{section.items.map(renderItem)}</ul>}
      </section>
    );
  }

  return (
    <nav aria-label={t("tree.navLabel")} className="grid h-full min-h-0 grid-cols-1 grid-rows-[minmax(0,1fr)] md:grid-cols-[11rem_minmax(0,1fr)] md:grid-rows-1">
      <aside className="hidden min-h-0 flex-col border-r border-line bg-tree md:flex">
        <p className="shrink-0 px-3 pb-1 pt-3 text-[0.68rem] font-semibold uppercase tracking-[0.12em] text-ink-faint">
          {t("tree.byGroups")}
        </p>
        <div data-connection-facets className="flex min-h-0 min-w-0 flex-1 flex-col gap-1 overflow-x-hidden overflow-y-auto px-3 pb-3">
          {renderFacets()}
        </div>
      </aside>

      <section className="flex min-h-0 flex-col bg-page" aria-label={t("tree.resultsLabel")}>
        <header className="shrink-0 border-b border-line bg-card p-3">
          <label className="mb-2 flex min-w-0 items-center gap-2 md:hidden">
            <span className="shrink-0 text-xs font-medium text-ink-muted">{t("tree.byGroups")}</span>
            <select
              aria-label={t("tree.groupFilter")}
              value={scopeValue}
              onChange={(event) => selectScope(event.currentTarget.value)}
              className={`${control} min-h-10 min-w-0 flex-1 py-2`}
            >
              <option value="all">{t("tree.allConnections")} · {decorated.length}</option>
              {orderedGroups.map((group) => (
                <option key={group.name} value={`group:${group.name}`}>{group.name} · {group.descendantCount}</option>
              ))}
              <option value="ungrouped">{t("tree.ungrouped")} · {decorated.filter((item) => item.group === "").length}</option>
            </select>
          </label>
          <div>
            <label className="relative block min-w-0" htmlFor="connection-filter">
              <span className="sr-only">{t("tree.filter")}</span>
              <Icon name="search" className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-ink-faint" />
              <input id="connection-filter" type="search" value={query} onChange={(event) => setQuery(event.currentTarget.value)} placeholder={t("tree.filterPlaceholderExpanded")} className={`${control} min-h-10 rounded-lg bg-control py-2 pl-9 pr-3 md:min-h-0`} />
            </label>
          </div>
          <div className="mt-2 flex items-center justify-between gap-3 text-xs text-ink-faint">
            <span role="status">{t("tree.resultCount", { visible: visible.length, total: decorated.length })}</span>
            <label className="flex items-center gap-2">
              <span className="sr-only">{t("tree.sortLabel")}</span>
              <select value={sortOrder} onChange={(event) => setSortOrder(event.currentTarget.value as SortOrder)} className="rounded border-0 bg-transparent py-0.5 text-xs text-ink-muted">
                <option value="configured">{t("tree.sortConfigured")}</option>
                <option value="name">{t("tree.sortName")}</option>
              </select>
            </label>
          </div>
          {!movesDisabled ? <p className="mt-1 hidden text-[0.68rem] text-ink-faint md:block">{t("tree.dragGroupHint")}</p> : null}
        </header>
        {visible.length === 0 ? <p role="status" className="p-5 text-sm text-ink-muted">{t("tree.noMatch")}</p> : <div data-connection-results className="min-h-0 flex-1 overflow-y-auto">{resultSections.map(renderResultSection)}</div>}
      </section>
    </nav>
  );
}
