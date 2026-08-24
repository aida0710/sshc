import { useMemo, useState, type ReactNode } from "react";
import type { Overview } from "../api/config";
import { buildConnectionBrowserIndex, identityKey, projectConnectionBrowser, type BrowserGroup, type BrowserServer, type ConnectionBrowserLocation } from "../connections/connectionBrowser";
import { useTranslate } from "../i18n/context";
import { control } from "../ui/form";
import { Segmented } from "../ui/surface";
import { ConnectionActions } from "./ConnectionActions";

type QuickConnectBrowserProps = {
  overview: Overview;
  launching: string;
  onConnect: (alias: string) => void;
  onOpenSettings: (location: string) => void;
};

export function QuickConnectBrowser({
  overview,
  launching,
  onConnect,
  onOpenSettings,
}: QuickConnectBrowserProps) {
  const t = useTranslate();
  const [browser, setBrowser] = useState<ConnectionBrowserLocation>({ view: "servers" });
  const [query, setQuery] = useState("");
  const [favouritesOnly, setFavouritesOnly] = useState(false);
  const index = useMemo(() => buildConnectionBrowserIndex(overview), [overview]);
  const projection = projectConnectionBrowser(index, browser, query, favouritesOnly);

  function countLabel(count: number) {
    return count === 1
      ? t("browser.groupCountOne")
      : t("browser.groupCountMany", { count });
  }

  function renderServers(source: BrowserServer[]): ReactNode {
    const servers = [...source].sort((left, right) => Number(right.favourite) - Number(left.favourite));
    if (servers.length === 0) return null;
    return (
      <ul aria-label={t("home.connectionList")} className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {servers.map((server) => (
          <li
            key={identityKey(server.identity)}
            className="flex min-w-0 flex-col gap-3 rounded-xl border border-line bg-card p-4"
          >
            <div className="flex min-w-0 items-start gap-2">
              <div className="min-w-0 grow">
                <p className="flex min-w-0 items-center gap-1 truncate font-medium text-ink">
                  {server.colour === "" ? null : (
                    <span
                      aria-hidden="true"
                      className="inline-block size-2 shrink-0 rounded-full"
                      style={{ backgroundColor: server.colour }}
                    />
                  )}
                  {server.favourite ? (
                    <span aria-label={t("home.favourite")} className="text-notice-ink">★</span>
                  ) : null}
                  <span className="truncate">{server.identity.alias}</span>
                  {server.duplicateAlias ? (
                    <span aria-label={t("browser.duplicateAlias")} className="text-notice-ink">⧉</span>
                  ) : null}
                </p>
                <p className="truncate text-xs text-ink-muted">
                  {server.group === "" ? t("home.ungrouped") : server.group}
                </p>
                {server.duplicateAlias ? (
                  <p className="truncate text-xs text-ink-faint">{server.identity.path}</p>
                ) : null}
              </div>
              <ConnectionActions
                alias={server.identity.alias}
                path={server.identity.path}
                busy={launching !== ""}
                opening={launching === server.identity.alias}
                onOpenSettings={onOpenSettings}
                onConnect={() => onConnect(server.identity.alias)}
              />
            </div>
            {server.tags.length === 0 ? null : (
              <ul aria-label={t("home.tagsFor", { alias: server.identity.alias })} className="flex flex-wrap gap-1">
                {server.tags.map((tag) => (
                  <li key={tag} className="rounded bg-select-fill px-2 py-0.5 text-xs text-ink-muted">{tag}</li>
                ))}
              </ul>
            )}
          </li>
        ))}
      </ul>
    );
  }

  function renderGroup(group: BrowserGroup) {
    return (
      <li key={group.name}>
        <button
          type="button"
          aria-label={`${group.label}, ${countLabel(group.descendantCount)}`}
          onClick={() => setBrowser({ view: "groups", scope: "named", group: group.name })}
          className="flex w-full items-center gap-3 rounded-xl border border-line bg-card p-4 text-left hover:bg-select-fill"
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
      <nav aria-label={t("browser.groupPath")} className="flex flex-wrap items-center gap-1 text-sm text-ink-muted">
        <button
          type="button"
          onClick={() => setBrowser({ view: "groups", scope: "root" })}
          className="rounded px-1.5 py-1 hover:bg-select-fill"
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
                    onClick={() => setBrowser({ view: "groups", scope: "named", group: group.name })}
                    className="rounded px-1.5 py-1 hover:bg-select-fill"
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
        <div className="rounded-xl border border-line bg-card p-5 text-sm">
          <p className="font-medium">{t("browser.groupMissing")}</p>
          <p className="mt-1 text-ink-muted">{t("home.groupMissingDetail", { name: projection.group })}</p>
          <button
            type="button"
            onClick={() => setBrowser({ view: "groups", scope: "root" })}
            className="mt-2 rounded px-2 py-1 text-accent hover:bg-select-fill"
          >
            {t("browser.backToGroupRoot")}
          </button>
        </div>
      );
    }
    if (projection.kind === "servers" || projection.kind === "search-results") {
      if (projection.servers.length > 0) return renderServers(projection.servers);
      const empty = index.servers.length === 0 ? t("home.noConnections") : t("home.noMatches");
      return <p className="rounded-xl border border-line bg-card p-5 text-sm text-ink-muted">{empty}</p>;
    }

    const atRoot = browser.view === "groups" && browser.scope === "root";
    const showUngrouped = atRoot && projection.ungroupedCount > 0;
    if (projection.groups.length === 0 && projection.servers.length === 0 && !showUngrouped) {
      const empty = favouritesOnly
        ? t("browser.noMatches")
        : atRoot
          ? t("browser.emptyGroups")
          : t("browser.emptyGroup");
      return <p className="rounded-xl border border-line bg-card p-5 text-sm text-ink-muted">{empty}</p>;
    }
    return (
      <div className="flex flex-col gap-3">
        {projection.groups.length === 0 && !showUngrouped ? null : (
          <ul aria-label={t("browser.groups")} className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
            {projection.groups.map(renderGroup)}
            {showUngrouped ? (
              <li>
                <button
                  type="button"
                  aria-label={`${t("browser.ungrouped")}, ${countLabel(projection.ungroupedCount)}`}
                  onClick={() => setBrowser({ view: "groups", scope: "ungrouped" })}
                  className="flex w-full items-center gap-3 rounded-xl border border-line bg-card p-4 text-left hover:bg-select-fill"
                >
                  <span className="min-w-0 flex-1 font-medium">{t("browser.ungrouped")}</span>
                  <span className="shrink-0 text-xs text-ink-faint">{countLabel(projection.ungroupedCount)}</span>
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
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="[&_button]:min-h-10 md:[&_button]:min-h-0">
          <Segmented
            label={t("browser.modeLabel")}
            value={browser.view}
            options={[
              { value: "servers", label: t("browser.servers") },
              { value: "groups", label: t("browser.groups") },
            ]}
            onChange={(view) => setBrowser(view === "servers" ? { view: "servers" } : { view: "groups", scope: "root" })}
          />
        </div>
        <label className="w-full sm:w-72">
          <span className="sr-only">{t("home.search")}</span>
          <input
            type="search"
            value={query}
            onChange={(event) => setQuery(event.currentTarget.value)}
            placeholder={t("home.searchPlaceholder")}
            className={`${control} min-h-10 md:min-h-0`}
          />
        </label>
      </div>
      <button
        type="button"
        aria-pressed={favouritesOnly}
        onClick={() => setFavouritesOnly((current) => !current)}
        className={`min-h-10 self-start rounded px-2 py-1 text-xs md:min-h-0 ${
          favouritesOnly ? "bg-select-fill text-ink" : "text-ink-muted hover:bg-card"
        }`}
      >
        {t("browser.favouritesOnly")}
      </button>
      {renderBreadcrumbs()}
      {renderProjection()}
    </div>
  );
}
