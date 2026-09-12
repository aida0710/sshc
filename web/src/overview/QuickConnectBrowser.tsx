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
import { Icon } from "../ui/icons";
import { OperatingSystemIcon } from "../ui/OperatingSystemIcon";
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
    const lastConnected = recentConnection === undefined
      ? ""
      : t("home.lastConnected", { at: formatConnectedAt(recentConnection.lastConnectedAt) });
    const selected = selectedAlias === alias;
    const opening = launching === alias;
    const panel = view === "panel";
    const connect = () => {
      if (launching !== "") return;
      setSelectedAlias(alias);
      onConnect(alias);
    };

    return (
      <li
        key={identityKey(server.identity)}
        aria-busy={opening}
        className={`relative min-w-0 rounded-lg border transition-colors ${
          selected || opening ? "border-accent bg-select-fill" : "border-line bg-card hover:border-control-line hover:bg-hover"
        }`}
      >
        <button
          type="button"
          data-touch-compact
          aria-label={t("home.connectGesture", { alias })}
          aria-pressed={selected}
          disabled={launching !== ""}
          onPointerDown={(event) => { pointerType.current = event.pointerType; }}
          onClick={() => {
            if (pointerType.current === "mouse") {
              setSelectedAlias(alias);
              return;
            }
            connect();
          }}
          onDoubleClick={() => {
            if (pointerType.current === "mouse") connect();
          }}
          onKeyDown={(event) => {
            if (event.key !== "Enter" && event.key !== " ") return;
            event.preventDefault();
            connect();
          }}
          className={`flex w-full min-w-0 items-center gap-3 rounded-lg py-3 pl-3 pr-24 text-left disabled:cursor-wait max-md:min-h-28 max-md:pr-28 [@media(pointer:coarse)]:min-h-28 [@media(pointer:coarse)]:pr-28 ${
            panel ? "min-h-24" : "min-h-20"
          }`}
        >
          <OperatingSystemIcon os={server.os} colour={server.colour} />
          <span className="min-w-0 flex-1">
            <span className="flex min-w-0 items-center gap-1.5">
              <span className="truncate text-sm font-semibold text-ink" title={alias}>{alias}</span>
              {server.duplicateAlias ? <span aria-label={t("browser.duplicateAlias")} className="text-notice-ink">⧉</span> : null}
            </span>
            <span className="mt-1 block truncate font-mono text-xs text-ink-muted" title={target}>{target}</span>
            {server.group === "" && lastConnected === "" ? null : (
              <span className="mt-1.5 flex min-w-0 items-center gap-1.5 text-[11px] text-ink-faint">
                {server.group === "" ? null : <span className="truncate" title={server.group}>{server.group}</span>}
                {server.group === "" || lastConnected === "" ? null : <span aria-hidden="true">·</span>}
                {lastConnected === "" ? null : <span className="truncate" title={lastConnected}>{lastConnected}</span>}
              </span>
            )}
          </span>
        </button>
        <div className="pointer-events-none absolute inset-y-2 right-2 flex flex-col items-end justify-between gap-1">
          <ConnectionActions
            alias={alias}
            path={server.identity.path}
            busy={launching !== ""}
            opening={opening}
            onOpenSettings={onOpenSettings}
            onConnect={connect}
          />
          <button
            type="button"
            aria-label={t(opening ? "home.openingConnection" : "home.connectTo", { alias })}
            disabled={launching !== ""}
            onClick={connect}
            className="pointer-events-auto inline-flex min-h-8 items-center justify-center gap-1.5 rounded-md border border-accent/35 bg-accent/10 px-2.5 py-1 text-xs font-medium text-accent hover:bg-accent/20 disabled:cursor-wait disabled:opacity-60"
          >
            {opening ? <span aria-hidden="true" className="size-3 animate-spin rounded-full border-2 border-current border-r-transparent motion-reduce:animate-none" /> : null}
            <span aria-live="polite">{opening ? t("home.opening") : t("home.connect")}</span>
          </button>
        </div>
      </li>
    );
  }

  return (
    <div className="flex flex-col gap-3">
      <div className="grid grid-cols-[minmax(0,1fr)_auto] gap-2">
        <label className="relative min-w-0">
          <span className="sr-only">{t("home.search")}</span>
          <Icon name="search" className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-ink-faint" />
          <input
            type="search"
            value={query}
            onChange={(event) => setQuery(event.currentTarget.value)}
            placeholder={t("home.searchPlaceholder")}
            className={`${control} min-h-10 w-full pl-9 md:min-h-0`}
          />
        </label>
        <div className="flex">
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

      {visibleGroups.length === 0 && groupTrail.length === 0 ? null : (
        <section aria-label={t("home.groups")} className="flex flex-col gap-2">
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
          {visibleGroups.length === 0 ? null : (
            <div role="group" aria-label={t("home.groupFilter")} className="flex flex-wrap gap-2">
              {visibleGroups.map((candidate) => {
                const childCount = index.visibleChildrenByParent.get(candidate.name)?.length ?? 0;
                return (
                  <button
                    key={candidate.name}
                    type="button"
                    title={candidate.name}
                    aria-label={t("home.openGroup", { name: candidate.name, count: candidate.descendantCount })}
                    onClick={() => setGroupTrail([...groupTrail, candidate.name])}
                    className="flex min-h-9 min-w-0 max-w-full items-center gap-2 rounded-md border border-line bg-card px-3 py-1.5 text-left transition-colors hover:border-control-line hover:bg-hover"
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
      )}

      <div className="flex items-center justify-between gap-3">
        <h3 className="text-xs font-semibold text-ink">{t("home.connectionCount", { count: servers.length })}</h3>
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
            ? "grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3"
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
