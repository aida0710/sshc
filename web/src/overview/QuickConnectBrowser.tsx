import { useEffect, useMemo, useRef, useState } from "react";
import type { Overview } from "../api/config";
import type { RecentConnection } from "../api/integrations";
import {
  buildConnectionBrowserIndex,
  identityKey,
  type BrowserServer,
} from "../connections/connectionBrowser";
import { useTranslate } from "../i18n/context";
import { control } from "../ui/form";
import { Segmented } from "../ui/surface";
import { ConnectionActions } from "./ConnectionActions";

type QuickConnectBrowserProps = {
  overview: Overview;
  recent?: RecentConnection[];
  launching: string;
  onConnect: (alias: string) => void;
  onOpenSettings: (location: string) => void;
};

type QuickConnectView = "panel" | "list";

const viewStorageKey = "sshc.home.quick-connect-view";

function storedView(): QuickConnectView {
  try {
    return window.localStorage.getItem(viewStorageKey) === "list" ? "list" : "panel";
  } catch {
    return "panel";
  }
}

function rememberView(view: QuickConnectView) {
  try {
    window.localStorage.setItem(viewStorageKey, view);
  } catch {
    // The launcher still works when storage is unavailable.
  }
}

function destination(hostName: string, user: string, port: string): string {
  const host = hostName.includes(":") && !hostName.startsWith("[") ? `[${hostName}]` : hostName;
  if (host === "") return "";
  return `${user === "" ? "" : `${user}@`}${host}${port === "" ? "" : `:${port}`}`;
}

function connectionDestination(server: BrowserServer): string {
  return destination(server.host.hostName ?? server.identity.alias, server.host.user ?? "", server.host.port ?? "22");
}

function includesQuery(server: BrowserServer, query: string): boolean {
  const needle = query.trim().toLocaleLowerCase();
  if (needle === "") return true;
  return [
    server.identity.alias,
    server.identity.path,
    server.group,
    connectionDestination(server),
    ...server.host.patterns,
    ...server.tags,
  ].some((candidate) => candidate.toLocaleLowerCase().includes(needle));
}

function belongsToGroup(server: BrowserServer, group: string): boolean {
  return group === "" || server.group === group || server.group.startsWith(`${group}/`);
}

export function QuickConnectBrowser({
  overview,
  recent = [],
  launching,
  onConnect,
  onOpenSettings,
}: QuickConnectBrowserProps) {
  const t = useTranslate();
  const [query, setQuery] = useState("");
  const [groupTrail, setGroupTrail] = useState<string[]>([]);
  const [view, setView] = useState<QuickConnectView>(storedView);
  const [selectedAlias, setSelectedAlias] = useState("");
  const pointerType = useRef("keyboard");
  const index = useMemo(() => buildConnectionBrowserIndex(overview), [overview]);
  const recentByAlias = useMemo(
    () => new Map(recent.map((connection, position) => [connection.alias, { connection, position }])),
    [recent],
  );
  const collator = useMemo(
    () => new Intl.Collator(undefined, { numeric: true, sensitivity: "base" }),
    [],
  );
  const group = groupTrail.at(-1) ?? "";
  const visibleGroups = index.visibleChildrenByParent.get(group) ?? [];

  useEffect(() => {
    if (groupTrail.every((name) => index.groupByName.has(name))) return;
    setGroupTrail([]);
  }, [groupTrail, index.groupByName]);

  const servers = useMemo(
    () => index.servers
      .filter((server) => belongsToGroup(server, group) && includesQuery(server, query))
      .sort((left, right) => {
        const leftRecent = recentByAlias.get(left.identity.alias)?.position;
        const rightRecent = recentByAlias.get(right.identity.alias)?.position;
        if (leftRecent !== undefined && rightRecent !== undefined) return leftRecent - rightRecent;
        if (leftRecent !== undefined) return -1;
        if (rightRecent !== undefined) return 1;
        return collator.compare(left.identity.alias, right.identity.alias);
      }),
    [collator, group, index.servers, query, recentByAlias],
  );

  function changeView(next: QuickConnectView) {
    setView(next);
    rememberView(next);
  }

  function renderServer(server: BrowserServer) {
    const alias = server.identity.alias;
    const recentConnection = recentByAlias.get(alias)?.connection;
    const target = connectionDestination(server);
    const location = server.group === "" ? t("home.ungrouped") : server.group;
    const lastConnected = recentConnection === undefined
      ? t("home.neverConnected")
      : t("home.lastConnected", { at: formatConnectedAt(recentConnection.lastConnectedAt) });
    const selected = selectedAlias === alias;
    const panel = view === "panel";

    return (
      <li
        key={identityKey(server.identity)}
        className={`relative min-w-0 border border-line bg-card transition-colors hover:bg-select-fill ${
          panel ? "rounded-md" : "rounded-sm"
        } ${selected ? "bg-select-fill" : ""}`}
      >
        <button
          type="button"
          aria-label={t("home.connectGesture", { alias })}
          disabled={launching !== ""}
          onPointerDown={(event) => { pointerType.current = event.pointerType; }}
          onClick={() => {
            if (pointerType.current === "mouse") {
              setSelectedAlias(alias);
              return;
            }
            onConnect(alias);
          }}
          onDoubleClick={() => {
            if (pointerType.current === "mouse") onConnect(alias);
          }}
          onKeyDown={(event) => {
            if (event.key !== "Enter" && event.key !== " ") return;
            event.preventDefault();
            onConnect(alias);
          }}
          className={`block min-h-24 w-full min-w-0 pr-12 text-left disabled:cursor-wait disabled:text-ink-muted ${
            panel ? "px-3 py-2.5" : "px-3 py-2"
          }`}
        >
          <span className="flex min-w-0 items-center gap-2">
            {server.colour === "" ? (
              <span aria-hidden="true" className="size-2 shrink-0 rounded-full bg-live" />
            ) : (
              <span aria-hidden="true" className="size-2 shrink-0 rounded-full" style={{ backgroundColor: server.colour }} />
            )}
            <span className="truncate text-sm font-semibold text-ink">{alias}</span>
            {server.duplicateAlias ? <span aria-label={t("browser.duplicateAlias")} className="text-notice-ink">⧉</span> : null}
          </span>
          <span className="mt-1 block truncate text-xs text-ink-muted">{location}</span>
          <span className="mt-0.5 block truncate font-mono text-xs text-ink">{target}</span>
          <span className="mt-1 block truncate text-xs text-ink-faint">
            {launching === alias ? t("home.opening") : lastConnected}
          </span>
        </button>
        <div className="absolute right-1.5 top-1.5">
          <ConnectionActions
            alias={alias}
            path={server.identity.path}
            busy={launching !== ""}
            opening={launching === alias}
            onOpenSettings={onOpenSettings}
            onConnect={() => onConnect(alias)}
          />
        </div>
      </li>
    );
  }

  return (
    <div className="flex flex-col gap-3">
      <div className="grid grid-cols-[minmax(0,1fr)_auto] gap-2">
        <label className="min-w-0">
          <span className="sr-only">{t("home.search")}</span>
          <input
            type="search"
            value={query}
            onChange={(event) => setQuery(event.currentTarget.value)}
            placeholder={t("home.searchPlaceholder")}
            className={`${control} min-h-10 w-full md:min-h-0`}
          />
        </label>
        <div className="[&_button]:min-h-10 md:[&_button]:min-h-0">
          <Segmented
            label={t("home.viewMode")}
            value={view}
            options={[
              { value: "panel", label: t("home.panelView") },
              { value: "list", label: t("home.listView") },
            ]}
            onChange={changeView}
          />
        </div>
      </div>

      <section aria-labelledby="quick-connect-groups-heading" className="flex flex-col gap-2">
        <div className="flex min-w-0 flex-wrap items-center justify-between gap-2">
          <h4 id="quick-connect-groups-heading" className="text-xs font-semibold text-ink">
            {t("home.groupCount", { count: visibleGroups.length })}
          </h4>
          {groupTrail.length === 0 ? null : (
            <nav aria-label={t("home.groupBreadcrumb")} className="flex min-w-0 items-center gap-1 text-xs text-ink-muted">
              <button type="button" onClick={() => setGroupTrail([])} className="shrink-0 rounded px-1 py-0.5 hover:bg-card hover:text-ink">
                {t("home.allGroups")}
              </button>
              {groupTrail.map((name, position) => {
                const selected = position === groupTrail.length - 1;
                return (
                  <span key={name} className="flex min-w-0 items-center gap-1">
                    <span aria-hidden="true" className="text-ink-faint">/</span>
                    <button
                      type="button"
                      aria-current={selected ? "page" : undefined}
                      onClick={() => setGroupTrail(groupTrail.slice(0, position + 1))}
                      className={`truncate rounded px-1 py-0.5 hover:bg-card hover:text-ink ${selected ? "font-medium text-ink" : ""}`}
                    >
                      {index.groupByName.get(name)?.label ?? name}
                    </button>
                  </span>
                );
              })}
            </nav>
          )}
        </div>
        {visibleGroups.length === 0 ? (
          <p className="text-xs text-ink-faint">{t("home.noChildGroups")}</p>
        ) : (
          <div role="group" aria-label={t("home.groupFilter")} className="grid grid-cols-2 gap-2 md:grid-cols-4">
            {visibleGroups.map((candidate) => {
              const childCount = index.visibleChildrenByParent.get(candidate.name)?.length ?? 0;
              return (
                <button
                  key={candidate.name}
                  type="button"
                  title={candidate.name}
                  aria-label={t("home.openGroup", { name: candidate.name, count: candidate.descendantCount })}
                  onClick={() => setGroupTrail([...groupTrail, candidate.name])}
                  className="flex min-h-12 min-w-0 items-center gap-2 rounded-md border border-line bg-surface-subtle px-3 py-2 text-left transition-colors hover:bg-select-fill focus-visible:bg-select-fill"
                >
                  <span
                    aria-hidden="true"
                    className={`size-2 shrink-0 rounded-full ${candidate.colour === "" ? "bg-accent" : ""}`}
                    style={candidate.colour === "" ? undefined : { backgroundColor: candidate.colour }}
                  />
                  <span className="min-w-0 flex-1 truncate text-xs font-medium text-ink">{candidate.label}</span>
                  <span className="shrink-0 font-mono text-xs tabular-nums text-ink-muted">{candidate.descendantCount}</span>
                  {childCount === 0 ? null : <span aria-hidden="true" className="shrink-0 text-ink-faint">›</span>}
                </button>
              );
            })}
          </div>
        )}
      </section>

      <div className="flex items-center justify-between gap-3">
        <h4 className="text-xs font-semibold text-ink">{t("home.connectionCount", { count: servers.length })}</h4>
        <span className="hidden text-right text-xs text-ink-faint md:inline">{t("home.pointerHint")}</span>
        <span className="text-right text-xs text-ink-faint md:hidden">{t("home.touchHint")}</span>
      </div>

      {servers.length === 0 ? (
        <p className="border-y border-line bg-surface-subtle p-4 text-sm text-ink-muted">
          {index.servers.length === 0 ? t("home.noConnections") : t("home.noMatches")}
        </p>
      ) : (
        <ul
          aria-label={t("home.connectionList")}
          className={view === "panel"
            ? "grid grid-cols-1 gap-2 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4"
            : "flex flex-col gap-1"}
        >
          {servers.map(renderServer)}
        </ul>
      )}
    </div>
  );
}

function formatConnectedAt(value: string): string {
  const connectedAt = new Date(value);
  if (Number.isNaN(connectedAt.valueOf())) return value;
  return new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(connectedAt);
}
