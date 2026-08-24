import { Fragment, useMemo, useState, type DragEvent, type ReactNode } from "react";
import type { HostEntry, Overview } from "../api/config";
import { useTranslate } from "../i18n/context";
import { control } from "../ui/form";
import { Segmented } from "../ui/surface";
import { duplicateAliasesOf, identityKey } from "./connectionBrowser";
import { canDrop, dragMimeType, type DragPayload } from "./dragdrop";

export type HostSelection = { path: string; alias: string };

type DecoratedHost = {
  host: HostEntry;
  group: string;
  tags: string[];
  favourite: boolean;
  colour: string;
  order: number;
  duplicateAlias: boolean;
};

type GroupNode = {
  name: string;
  label: string;
  hidden: boolean;
  order: number;
  sourceOrder: number;
  items: DecoratedHost[];
  children: GroupNode[];
};

type ConnectionTreeProps = {
  overview: Overview;
  selected: HostSelection | null;
  onSelect: (host: HostEntry) => void;
  onOpenPatternRule: (path: string, line: number) => void;
  onDrop: (payload: DragPayload, target: string) => void;
  movesDisabled?: boolean;
};

type Grouping = "groups" | "files";

function labelFor(host: HostEntry): string {
  return host.identity.alias === "" ? `Host ${host.patterns.join(" ")}` : host.identity.alias;
}

function hostMatches(item: DecoratedHost, rawQuery: string): boolean {
  const query = rawQuery.trim().toLocaleLowerCase();
  if (query === "") return true;
  return item.host.identity.alias.toLocaleLowerCase().includes(query) ||
    item.host.patterns.some((pattern) => pattern.toLocaleLowerCase().includes(query)) ||
    item.group.toLocaleLowerCase().includes(query) ||
    item.tags.some((tag) => tag.toLocaleLowerCase().includes(query));
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

export function ConnectionTree({
  overview,
  selected,
  onSelect,
  onOpenPatternRule,
  onDrop,
  movesDisabled = false,
}: ConnectionTreeProps) {
  const t = useTranslate();
  const [grouping, setGrouping] = useState<Grouping>("groups");
  const [query, setQuery] = useState("");
  const [favouritesOnly, setFavouritesOnly] = useState(false);
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(new Set());
  const [dragging, setDragging] = useState<DragPayload | null>(null);

  const groupNames = useMemo(() => overview.groups.map((group) => group.name), [overview.groups]);
  const decorated = useMemo<DecoratedHost[]>(() => {
    const metadata = new Map(
      (overview.metadata.hosts ?? []).map((entry) => [identityKey(entry.identity), entry]),
    );
    const duplicates = duplicateAliasesOf(overview.hosts);
    return overview.hosts
      .map((host, sourceOrder) => {
        const display = metadata.get(identityKey(host.identity));
        return {
          host,
          group: host.group ?? "",
          tags: display?.tags ?? [],
          favourite: display?.favourite === true,
          colour: display?.colour ?? "",
          order: display?.order ?? 0,
          duplicateAlias: duplicates.has(host.identity.alias),
          sourceOrder,
        };
      })
      .sort((left, right) => left.order - right.order || left.sourceOrder - right.sourceOrder)
      .map(({ sourceOrder: _sourceOrder, ...item }) => item);
  }, [overview.hosts, overview.metadata.hosts]);

  const visible = decorated.filter(
    (item) => (!favouritesOnly || item.favourite) && hostMatches(item, query),
  );

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
        items: visible.filter((item) => item.group === group.name),
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
  }, [overview.groups, overview.metadata.groups, visible]);

  const fileSections = useMemo(
    () => overview.files.map((file) => ({
      title: file.file.path ?? file.file.absolute,
      items: visible.filter((item) => item.host.file.absolute === file.file.absolute),
    })),
    [overview.files, visible],
  );
  const ungrouped = visible.filter((item) => item.group === "");

  function startDrag(event: DragEvent, payload: DragPayload) {
    if (movesDisabled || grouping !== "groups") return;
    event.dataTransfer.setData(dragMimeType, JSON.stringify(payload));
    event.dataTransfer.effectAllowed = "move";
    setDragging(payload);
  }

  function accepts(target: string): boolean {
    return !movesDisabled && grouping === "groups" && dragging !== null &&
      canDrop(dragging, target, groupNames);
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

  function renderItems(items: DecoratedHost[]): ReactNode {
    return (
      <ul className="flex flex-col gap-1.5">
        {items.map((item) => {
          const host = item.host;
          const active = selected?.path === host.identity.path && selected.alias === host.identity.alias;
          const descriptionId = `connection-${host.file.absolute}-${host.line}`;
          if (host.identity.alias === "") {
            return (
              <li key={`${host.file.absolute}:${host.line}`}>
                {host.file.path === undefined ? (
                  <p aria-describedby={descriptionId} className="rounded px-2 py-1 text-sm text-ink-muted">
                    <span className="block">{labelFor(host)}</span>
                    <span className="block text-xs text-ink-faint">
                      {t("tree.patternRuleExternal", { path: host.file.absolute })}
                    </span>
                  </p>
                ) : (
                  <button
                    type="button"
                    aria-describedby={descriptionId}
                    onClick={() => onOpenPatternRule(host.file.path ?? "", host.line)}
                    className="w-full rounded px-2 py-1 text-left text-sm hover:bg-select-fill"
                  >
                    <span className="block">{labelFor(host)}</span>
                    <span className="block text-xs text-ink-faint">
                      {t("tree.patternRuleOpen", { path: host.file.path, line: host.line })}
                    </span>
                  </button>
                )}
                <span id={descriptionId} className="sr-only">{t("tree.patternRule")}</span>
              </li>
            );
          }
          return (
            <li key={`${host.file.absolute}:${host.line}`}>
              <button
                type="button"
                aria-label={host.identity.alias}
                aria-current={active ? "true" : undefined}
                aria-describedby={descriptionId}
                draggable={grouping === "groups" && !movesDisabled}
                onClick={() => onSelect(host)}
                onDragStart={(event) => startDrag(event, {
                  kind: "connection",
                  path: host.identity.path,
                  alias: host.identity.alias,
                  group: item.group,
                })}
                onDragEnd={() => setDragging(null)}
                className={`w-full rounded-lg px-3 py-2.5 text-left text-sm transition-all ${
                  active
                    ? "bg-card text-ink shadow-sm ring-1 ring-control-line"
                    : "text-ink-muted hover:bg-card hover:text-ink"
                }`}
              >
                <span className="flex min-w-0 items-center gap-1.5">
                  {item.colour === "" ? null : (
                    <span aria-hidden="true" className="inline-block size-2 shrink-0 rounded-full" style={{ backgroundColor: item.colour }} />
                  )}
                  {item.favourite ? <span aria-hidden="true" className="text-notice-ink">★</span> : null}
                  <span className={`truncate ${active ? "font-semibold" : "font-medium"}`}>{host.identity.alias}</span>
                  {item.duplicateAlias ? <span aria-hidden="true" className="text-notice-ink">⧉</span> : null}
                </span>
                <span className="mt-1 block truncate font-mono text-[0.68rem] leading-4 text-ink-faint">
                  {host.file.path ?? host.file.absolute}
                </span>
                {item.tags.length === 0 ? null : (
                  <span aria-hidden="true" className="mt-1 flex flex-wrap gap-1">
                    {item.tags.map((tag) => (
                      <span key={tag} className="rounded bg-select-fill px-1 text-[0.65rem] text-ink-muted">{tag}</span>
                    ))}
                  </span>
                )}
              </button>
              <span id={descriptionId} className="sr-only">
                {[
                  item.favourite ? t("tree.favourite") : "",
                  item.duplicateAlias ? t("tree.duplicateAlias") : "",
                  host.file.path ?? host.file.absolute,
                ].filter(Boolean).join(", ")}
              </span>
            </li>
          );
        })}
      </ul>
    );
  }

  function renderGroup(node: GroupNode): ReactNode {
    if (node.hidden && node.items.length === 0) {
      return <Fragment key={node.name}>{node.children.map(renderGroup)}</Fragment>;
    }
    const shut = collapsed.has(node.name);
    return (
      <section
        key={node.name}
        aria-label={node.name}
        {...dropHandlers(node.name)}
      className={`flex flex-col gap-1 rounded-lg ${accepts(node.name) ? "bg-select-fill outline outline-1 outline-accent" : ""}`}
      >
        <div className="flex items-center gap-1">
          {node.children.length === 0 ? null : (
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
              className="rounded px-1 text-xs text-ink-faint hover:text-ink"
            >
              <span aria-hidden="true">{shut ? "▸" : "▾"}</span>
            </button>
          )}
          <h2
            draggable={!movesDisabled}
            onDragStart={(event) => startDrag(event, { kind: "group", name: node.name })}
            onDragEnd={() => setDragging(null)}
            className={`${movesDisabled ? "cursor-default" : "cursor-grab active:cursor-grabbing"} rounded px-1 text-[0.68rem] font-semibold uppercase tracking-[0.12em] text-ink-faint`}
          >
            <span aria-hidden="true" className="me-1 font-normal tracking-tighter">⋮⋮</span>
            {node.label}
          </h2>
        </div>
        {shut ? null : (
          <>
            {node.items.length > 0
              ? renderItems(node.items)
              : node.children.length === 0
                ? <p className="px-2 py-1 text-xs text-ink-faint">{t("tree.groupEmpty")}</p>
                : null}
            {node.children.length === 0 ? null : (
              <div className="ms-2 flex flex-col gap-1 border-s border-line ps-2">
                {node.children.map(renderGroup)}
              </div>
            )}
          </>
        )}
      </section>
    );
  }

  return (
    <nav aria-label={t("tree.navLabel")} className="flex h-full flex-col gap-4">
      <div className="flex items-center justify-between gap-2">
        <Segmented
          label={t("tree.arrangeBy")}
          value={grouping}
          options={[
            { value: "groups", label: t("tree.byGroups") },
            { value: "files", label: t("tree.byFiles") },
          ]}
          onChange={setGrouping}
        />
        <button
          type="button"
          aria-pressed={favouritesOnly}
          onClick={() => setFavouritesOnly((current) => !current)}
          className={`rounded-md border px-2.5 py-1 text-xs ${
            favouritesOnly
              ? "border-notice-line bg-notice text-notice-ink"
              : "border-control-line bg-card text-ink-muted hover:text-ink"
          }`}
        >
          <span aria-hidden="true" className="me-1 text-notice-ink">★</span>
          {t("tree.favouritesOnly")}
        </button>
      </div>
      <div className="flex flex-col gap-1.5">
        <label className="text-[0.68rem] font-semibold uppercase tracking-[0.12em] text-ink-faint" htmlFor="connection-filter">{t("tree.filter")}</label>
        <input
          id="connection-filter"
          type="search"
          value={query}
          onChange={(event) => setQuery(event.currentTarget.value)}
          placeholder={t("tree.filterPlaceholder")}
          className={`${control} rounded-lg bg-card px-3 py-2`}
        />
        {grouping === "groups" && groupTree.length > 0 && !movesDisabled ? (
          <p className="text-xs text-ink-faint">{t("tree.dragGroupHint")}</p>
        ) : null}
      </div>
      {visible.length === 0 ? <p role="status" className="text-sm text-ink-muted">{t("tree.noMatch")}</p> : null}
      {grouping === "files" ? (
        fileSections.map((section) => section.items.length === 0 ? null : (
          <section key={section.title} className="flex flex-col gap-1">
            <h2 className="rounded px-1 text-[0.68rem] font-semibold uppercase tracking-[0.12em] text-ink-faint">{section.title}</h2>
            {renderItems(section.items)}
          </section>
        ))
      ) : (
        <>
          {groupTree.map(renderGroup)}
          <section
            aria-label={t("tree.ungrouped")}
            {...dropHandlers("")}
            className={`flex flex-col gap-1 rounded-lg ${accepts("") ? "bg-select-fill outline outline-1 outline-accent" : ""}`}
          >
            <h2 className="rounded px-1 text-[0.68rem] font-semibold uppercase tracking-[0.12em] text-ink-faint">{t("tree.ungrouped")}</h2>
            {ungrouped.length === 0
              ? <p className="px-2 py-1 text-xs text-ink-faint">{t("tree.groupEmpty")}</p>
              : renderItems(ungrouped)}
          </section>
        </>
      )}
    </nav>
  );
}
