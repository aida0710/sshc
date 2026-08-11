import { useMemo, useState, type DragEvent, type ReactNode } from "react";
import type { HostEntry, Overview } from "../api/config";
import { useTranslate } from "../i18n/context";
import type { ConnectionBrowserLocation } from "../routing/connectionRoute";
import { control } from "../ui/form";
import { Segmented } from "../ui/surface";
import {
  buildConnectionBrowserIndex,
  projectConnectionBrowser,
  type BrowserGroup,
  type BrowserServer,
} from "./connectionBrowser";
import { canDrop, dragMimeType, type DragPayload } from "./dragdrop";

export type HostSelection = { path: string; alias: string };

type ConnectionBrowserProps = {
  overview: Overview;
  browser: ConnectionBrowserLocation;
  selected: HostSelection | null;
  movesDisabled: boolean;
  onBrowse: (browser: ConnectionBrowserLocation) => void;
  onSelect: (host: HostEntry) => void;
  onDrop: (payload: DragPayload, target: string) => void;
};

export function ConnectionBrowser({
  overview,
  browser,
  selected,
  movesDisabled,
  onBrowse,
  onSelect,
  onDrop,
}: ConnectionBrowserProps) {
  const t = useTranslate();
  const [query, setQuery] = useState("");
  const [favouritesOnly, setFavouritesOnly] = useState(false);
  const [dragging, setDragging] = useState<DragPayload | null>(null);
  const index = useMemo(() => buildConnectionBrowserIndex(overview), [overview]);
  const projection = projectConnectionBrowser(index, browser, query, favouritesOnly);
  const groupNames = useMemo(() => overview.groups.map((group) => group.name), [overview.groups]);
  const allowsMoves =
    browser.view === "groups" && projection.kind === "group-level" && !movesDisabled;

  function startDrag(event: DragEvent, payload: DragPayload) {
    if (!allowsMoves) return;
    event.dataTransfer.setData(dragMimeType, JSON.stringify(payload));
    event.dataTransfer.effectAllowed = "move";
    setDragging(payload);
  }

  function accepts(target: string): boolean {
    return allowsMoves && dragging !== null && canDrop(dragging, target, groupNames);
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

  function countLabel(count: number) {
    return count === 1
      ? t("browser.groupCountOne")
      : t("browser.groupCountMany", { count });
  }

  function renderServers(servers: BrowserServer[]): ReactNode {
    if (servers.length === 0) return null;
    return (
      <ul aria-label={t("browser.servers")} className="flex flex-col gap-1">
        {servers.map((server) => {
          const active =
            selected !== null &&
            selected.path === server.identity.path &&
            selected.alias === server.identity.alias;
          return (
            <li key={`${server.identity.path}\u0000${server.identity.alias}`}>
              <button
                type="button"
                onClick={() => onSelect(server.host)}
                aria-label={[
                  server.identity.alias,
                  server.favourite ? t("browser.favourite") : "",
                  server.duplicateAlias ? t("browser.duplicateAlias") : "",
                  server.group,
                  server.duplicateAlias ? server.identity.path : "",
                ]
                  .filter((part) => part !== "")
                  .join(", ")}
                draggable={allowsMoves}
                onDragStart={(event) =>
                  startDrag(event, {
                    kind: "connection",
                    path: server.identity.path,
                    alias: server.identity.alias,
                    group: server.group,
                  })
                }
                onDragEnd={() => setDragging(null)}
                aria-current={active ? "true" : undefined}
                className={`w-full rounded-lg border px-2.5 py-2 text-left text-sm transition-colors ${
                  active
                    ? "border-control-line bg-select-fill shadow-sm"
                    : "border-transparent hover:border-line hover:bg-card"
                }`}
              >
                <span className="flex min-w-0 items-center gap-1.5">
                  {server.colour === "" ? null : (
                    <span
                      aria-hidden="true"
                      className="inline-block size-2 shrink-0 rounded-full"
                      style={{ backgroundColor: server.colour }}
                    />
                  )}
                  {server.favourite ? (
                    <span aria-label={t("browser.favourite")} className="text-notice-ink">
                      ★
                    </span>
                  ) : null}
                  <span className="truncate font-medium">{server.identity.alias}</span>
                  {server.duplicateAlias ? (
                    <span aria-label={t("browser.duplicateAlias")} className="text-notice-ink">
                      ⧉
                    </span>
                  ) : null}
                </span>
                {server.group === "" ? null : (
                  <span className="mt-0.5 block truncate text-xs text-ink-faint">{server.group}</span>
                )}
                {server.duplicateAlias ? (
                  <span className="block truncate text-xs text-ink-faint">{server.identity.path}</span>
                ) : null}
                {server.tags.length === 0 ? null : (
                  <span aria-hidden="true" className="mt-1 flex flex-wrap gap-1">
                    {server.tags.map((tag) => (
                      <span key={tag} className="rounded bg-select-fill px-1 text-[0.65rem] text-ink-muted">
                        {tag}
                      </span>
                    ))}
                  </span>
                )}
              </button>
            </li>
          );
        })}
      </ul>
    );
  }

  function renderGroupRow(group: BrowserGroup) {
    const activeDrop = accepts(group.name);
    return (
      <li key={group.name}>
        <button
          type="button"
          aria-label={`${group.label}, ${countLabel(group.descendantCount)}`}
          draggable={allowsMoves}
          onDragStart={(event) => startDrag(event, { kind: "group", name: group.name })}
          onDragEnd={() => setDragging(null)}
          onClick={() => onBrowse({ view: "groups", scope: "named", group: group.name })}
          {...dropHandlers(group.name)}
          className={`flex w-full items-center gap-2 rounded-lg border px-2.5 py-2 text-left text-sm ${
            activeDrop
              ? "border-accent bg-select-fill"
              : "border-transparent hover:border-line hover:bg-card"
          }`}
        >
          {group.colour === "" ? null : (
            <span
              aria-hidden="true"
              className="size-2 shrink-0 rounded-full"
              style={{ backgroundColor: group.colour }}
            />
          )}
          <span className="min-w-0 flex-1 truncate font-medium">{group.label}</span>
          <span className="shrink-0 text-xs text-ink-faint">{countLabel(group.descendantCount)}</span>
          <span aria-hidden="true" className="text-ink-faint">›</span>
        </button>
      </li>
    );
  }

  function groupAncestors(group: string): BrowserGroup[] {
    const result: BrowserGroup[] = [];
    let current = index.groupByName.get(group);
    while (current !== undefined) {
      result.unshift(current);
      current = current.parent === "" ? undefined : index.groupByName.get(current.parent);
    }
    return result;
  }

  function renderBreadcrumbs() {
    if (browser.view !== "groups" || browser.scope === "root") return null;
    const ancestors = browser.scope === "named" ? groupAncestors(browser.group) : [];
    return (
      <nav aria-label={t("browser.groupPath")} className="flex flex-wrap items-center gap-1 text-xs text-ink-muted">
        <button
          type="button"
          onClick={() => onBrowse({ view: "groups", scope: "root" })}
          {...dropHandlers("")}
          className={`rounded px-1 py-0.5 hover:bg-select-fill ${accepts("") ? "bg-select-fill outline outline-1 outline-accent" : ""}`}
        >
          {t("browser.groups")}
        </button>
        {browser.scope === "ungrouped" ? (
          <>
            <span aria-hidden="true">/</span>
            <span aria-current="page">{t("browser.ungrouped")}</span>
          </>
        ) : (
          ancestors.map((group, indexInPath) => {
            const current = indexInPath === ancestors.length - 1;
            return (
              <span key={group.name} className="contents">
                <span aria-hidden="true">/</span>
                {current ? (
                  <span aria-current="page">{group.label}</span>
                ) : (
                  <button
                    type="button"
                    onClick={() => onBrowse({ view: "groups", scope: "named", group: group.name })}
                    {...dropHandlers(group.name)}
                    className={`rounded px-1 py-0.5 hover:bg-select-fill ${accepts(group.name) ? "bg-select-fill outline outline-1 outline-accent" : ""}`}
                  >
                    {group.label}
                  </button>
                )}
              </span>
            );
          })
        )}
      </nav>
    );
  }

  function renderProjection() {
    if (projection.kind === "missing-group") {
      return (
        <div className="flex flex-col gap-2 rounded-lg border border-line bg-card p-3 text-sm">
          <p className="font-medium">{t("browser.groupMissing")}</p>
          <p className="text-xs text-ink-muted">
            {t("browser.groupMissingDetail", { name: projection.group })}
          </p>
          <button
            type="button"
            onClick={() => onBrowse({ view: "groups", scope: "root" })}
            className="self-start rounded px-2 py-1 text-accent hover:bg-select-fill"
          >
            {t("browser.backToGroupRoot")}
          </button>
        </div>
      );
    }
    if (projection.kind === "servers") {
      if (projection.servers.length > 0) return renderServers(projection.servers);
      return (
        <p className="px-2 py-3 text-sm text-ink-faint">
          {index.servers.length === 0 ? t("browser.emptyServers") : t("browser.noMatches")}
        </p>
      );
    }
    if (projection.kind === "search-results") {
      return projection.servers.length > 0 ? (
        renderServers(projection.servers)
      ) : (
        <p className="px-2 py-3 text-sm text-ink-faint">{t("browser.noMatches")}</p>
      );
    }

    const atGroupRoot = browser.view === "groups" && browser.scope === "root";
    const showUngrouped = atGroupRoot && projection.ungroupedCount > 0;
    if (projection.groups.length === 0 && projection.servers.length === 0 && !showUngrouped) {
      const filtered = favouritesOnly;
      const emptyText = filtered
        ? t("browser.noMatches")
        : atGroupRoot
          ? t("browser.emptyGroups")
          : t("browser.emptyGroup");
      return <p className="px-2 py-3 text-sm text-ink-faint">{emptyText}</p>;
    }
    return (
      <div className="flex flex-col gap-2">
        {projection.groups.length === 0 && !showUngrouped ? null : (
          <ul aria-label={t("browser.groups")} className="flex flex-col gap-1">
            {projection.groups.map((group) => renderGroupRow(group))}
            {showUngrouped ? (
              <li>
                <button
                  type="button"
                  aria-label={`${t("browser.ungrouped")}, ${countLabel(projection.ungroupedCount)}`}
                  onClick={() => onBrowse({ view: "groups", scope: "ungrouped" })}
                  {...dropHandlers("")}
                  className={`flex w-full items-center gap-2 rounded-lg border px-2.5 py-2 text-left text-sm ${
                    accepts("") ? "border-accent bg-select-fill" : "border-transparent hover:border-line hover:bg-card"
                  }`}
                >
                  <span className="min-w-0 flex-1 font-medium">{t("browser.ungrouped")}</span>
                  <span className="text-xs text-ink-faint">{countLabel(projection.ungroupedCount)}</span>
                  <span aria-hidden="true" className="text-ink-faint">›</span>
                </button>
              </li>
            ) : null}
          </ul>
        )}
        {renderServers(projection.servers)}
      </div>
    );
  }

  return (
    <nav aria-label={t("browser.navLabel")} className="flex min-h-0 flex-1 flex-col gap-2">
      <Segmented
        label={t("browser.modeLabel")}
        value={browser.view}
        options={[
          { value: "servers", label: t("browser.servers") },
          { value: "groups", label: t("browser.groups") },
        ]}
        onChange={(view) =>
          onBrowse(view === "servers" ? { view: "servers" } : { view: "groups", scope: "root" })
        }
      />
      <label className="sr-only" htmlFor="connection-browser-filter">
        {t("browser.filter")}
      </label>
      <input
        id="connection-browser-filter"
        type="search"
        value={query}
        onChange={(event) => setQuery(event.currentTarget.value)}
        placeholder={t("browser.filterPlaceholder")}
        className={control}
      />
      <button
        type="button"
        aria-pressed={favouritesOnly}
        onClick={() => setFavouritesOnly((current) => !current)}
        className={`self-start rounded px-2 py-1 text-xs ${favouritesOnly ? "bg-select-fill text-ink" : "text-ink-muted hover:bg-card"}`}
      >
        {t("browser.favouritesOnly")}
      </button>
      {renderBreadcrumbs()}
      <div className="min-h-0 overflow-y-auto">
        {renderProjection()}
      </div>
    </nav>
  );
}
