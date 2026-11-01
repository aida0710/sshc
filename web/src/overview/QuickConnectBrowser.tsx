import { useEffect, useMemo, useRef, useState } from "react";
import { isGroupOrDescendant } from "../rules/groupPath";
import type { Overview } from "../api/config";
import type { RecentConnection } from "../api/recentConnections";
import {
  buildConnectionBrowserIndex,
  identityKey,
  type BrowserServer,
} from "../connections/connectionBrowser";
import { useLanguage } from "../i18n/context";
import { control } from "../ui/form";
import { Segmented } from "../ui/surface";
import { Icon } from "../ui/icons";
import { OperatingSystemIcon } from "../ui/OperatingSystemIcon";
import { localStorageKeys } from "../ui/browserStorageKeys";
import { useStoredValue, type StoredValue } from "../ui/useStoredValue";
import { ConnectionActions } from "./ConnectionActions";
import { hostMatchesQuery, normalizeHostQuery } from "../connections/hostSearch";
import { connectionAddress } from "../connections/connectionAddress";
import { revealOnHover } from "../ui/ActionMenu";
import { Spinner } from "../ui/Spinner";
import { formatDateTime } from "../ui/format";
import { relativeDay } from "../ui/relativeDay";
import { mobileViewportQuery, useMediaQuery } from "../ui/useMediaQuery";
import { literalTextEntry } from "../ui/softKeyboard";

type QuickConnectBrowserProps = {
  overview: Overview;
  recent?: RecentConnection[];
  launching: string;
  onConnect: (alias: string) => void;
  onOpenSettings: (location: string) => void;
};

type QuickConnectView = "panel" | "list";

// homeView is the view this browser shows Home in; the panels are the default.
const homeView: StoredValue<QuickConnectView> = {
  key: localStorageKeys.quickConnectView,
  fallback: "panel",
  parse: (stored) => stored === "list" ? "list" : "panel",
};

function connectionDestination(server: BrowserServer): string {
  return connectionAddress({ hostName: server.host.hostName ?? server.identity.alias, user: server.host.user ?? "", port: server.host.port ?? "" });
}

function includesQuery(server: BrowserServer, query: string): boolean {
  return hostMatchesQuery({
    alias: server.identity.alias, path: server.identity.path, group: server.group,
    destination: connectionDestination(server), patterns: server.host.patterns, tags: server.tags,
  }, normalizeHostQuery(query));
}

function belongsToGroup(server: BrowserServer, group: string): boolean {
  return group === "" || isGroupOrDescendant(server.group, group);
}

export function QuickConnectBrowser({
  overview,
  recent = [],
  launching,
  onConnect,
  onOpenSettings,
}: QuickConnectBrowserProps) {
  const { t, locale } = useLanguage();
  const mobile = useMediaQuery(mobileViewportQuery);
  const [query, setQuery] = useState("");
  const [groupTrail, setGroupTrail] = useState<string[]>([]);
  const [view, changeView] = useStoredValue(homeView);
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

  function renderServer(server: BrowserServer) {
    const alias = server.identity.alias;
    const recentConnection = recentByAlias.get(alias)?.connection;
    const target = connectionDestination(server);
    // The last connection is a badge beside the name, so a card that has one is as tall as one that has not.
    const lastConnected = recentConnection === undefined
      ? null
      : {
          day: relativeDay(recentConnection.lastConnectedAt, new Date(), locale),
          full: t("connection.lastConnected", { at: formatDateTime(recentConnection.lastConnectedAt, locale) }),
        };
    const selected = selectedAlias === alias;
    const opening = launching === alias;
    const connect = () => {
      if (launching !== "") return;
      setSelectedAlias(alias);
      onConnect(alias);
    };

    return (
      <li
        key={identityKey(server.identity)}
        aria-busy={opening}
        // A right click or a long press opens the card's menu: on a PC with a touch screen the 「…」 shows only
        // under a pointer, and a tap on the card connects.
        onContextMenu={(event) => {
          const trigger = event.currentTarget.querySelector<HTMLButtonElement>('button[aria-haspopup="menu"]');
          if (trigger === null || trigger.getAttribute("aria-expanded") === "true") return;
          event.preventDefault();
          trigger.click();
        }}
        className={`group/row relative min-w-0 rounded-xl border transition-colors ${
          selected || opening ? "border-accent bg-select-fill" : "border-transparent bg-well hover:border-accent/45"
        }`}
      >
        <button
          type="button"
          data-touch-compact
          aria-label={t(opening ? "home.openingConnection" : "home.connectGesture", { alias })}
          aria-pressed={selected}
          aria-busy={opening}
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
          className="flex w-full min-w-0 items-center gap-3 rounded-xl p-3.5 pr-12 text-left disabled:cursor-wait hover-reveal:pr-3.5"
        >
          <OperatingSystemIcon os={server.os} colour={server.colour} look="accentTile" />
          <span className="min-w-0 flex-1">
            <span className="flex min-w-0 items-center gap-1.5">
              <span className="truncate text-[13.5px] font-semibold text-ink" title={alias}>{alias}</span>
              {lastConnected === null || opening ? null : (
                <span title={lastConnected.full} aria-label={lastConnected.full}
                  className="shrink-0 whitespace-nowrap rounded-full bg-select-fill px-2 text-[11px] leading-[18px] text-accent">{lastConnected.day}</span>
              )}
              {server.duplicateAlias ? <span role="img" aria-label={t("browser.duplicateAlias")} className="text-notice-ink"><Icon name="warning" className="size-3.5" /></span> : null}
            </span>
            <span className="mt-0.5 block truncate font-mono text-[11.5px] text-ink-faint" title={target}>{target}</span>
            <span aria-live="polite">
              {opening ? (
                <span className="mt-1 flex items-center gap-1.5 text-[11px] text-ink-muted">
                  <Spinner className="size-3" />
                  {t("home.opening")}
                </span>
              ) : null}
            </span>
          </span>
          {/* Where the "…" shows only under the pointer (hover-reveal), it appears over the group name; elsewhere the
              card keeps room for it. A connection in no group shows no name: "no group" would only fill the room. */}
          {server.group === "" ? null : (
            <span
              className="max-w-[30%] shrink-0 truncate text-[11px] text-ink-faint transition-opacity duration-150 hover-reveal:group-hover/row:opacity-0 hover-reveal:group-focus-within/row:opacity-0"
              title={server.group}
            >
              {server.group}
            </span>
          )}
        </button>
        <div className={`pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 ${revealOnHover}`}>
          <ConnectionActions
            alias={alias}
            path={server.identity.path}
            busy={launching !== ""}
            opening={opening}
            onOpenSettings={onOpenSettings}
            onConnect={connect}
          />
        </div>
      </li>
    );
  }

  // On a phone the search takes the whole row, so its example is not cut; the view switch sits small at the
  // end of the count's row instead.
  const viewSwitch = (
    <Segmented
      label={t("home.viewMode")}
      value={view}
      options={[
        { value: "panel", label: t("home.panelView") },
        { value: "list", label: t("home.listView") },
      ]}
      onChange={changeView}
      touchCompact={mobile}
    />
  );

  return (
    <div className="flex flex-col gap-3">
      <div className={mobile ? "flex" : "grid grid-cols-[minmax(0,1fr)_auto] gap-2"}>
        <label className="relative min-w-0 flex-1">
          <span className="sr-only">{t("home.search")}</span>
          <Icon name="search" className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-ink-faint" />
          <input {...literalTextEntry}
            type="search"
            value={query}
            onChange={(event) => setQuery(event.currentTarget.value)}
            placeholder={t("hostSearch.placeholder")}
            className={`${control} min-h-10 w-full pl-8 md:min-h-0`}
          />
        </label>
        {mobile ? null : <div className="flex">{viewSwitch}</div>}
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
                    className="flex min-h-9 min-w-0 max-w-full items-center gap-2 rounded-md bg-well px-3 py-1.5 text-left transition-colors hover:bg-hover"
                  >
                    <span
                      aria-hidden="true"
                      className={`size-2 shrink-0 rounded-full ${candidate.colour === "" ? "bg-accent" : ""}`}
                      style={candidate.colour === "" ? undefined : { backgroundColor: candidate.colour }}
                    />
                    <span className="min-w-0 flex-1 truncate text-xs font-medium text-ink">{candidate.label}</span>
                    <span className="shrink-0 font-mono text-xs tabular-nums text-ink-muted">{candidate.descendantCount}</span>
                    {childCount === 0 ? null : <Icon name="chevronRight" className="size-3.5 text-ink-faint" />}
                  </button>
                );
              })}
            </div>
          )}
        </section>
      )}

      <div className="flex items-center gap-2">
        <h3 className="min-w-0 flex-1 text-xs font-normal text-ink-faint">{t("tree.resultCount", { visible: servers.length, total: index.servers.length })}</h3>
        {mobile ? viewSwitch : null}
      </div>

      {servers.length === 0 ? (
        <p className="rounded-[10px] bg-well p-4 text-sm text-ink-muted">
          {index.servers.length === 0 ? t("home.noConnections") : t("home.noMatches")}
        </p>
      ) : (
        <ul
          aria-label={t("home.connectionList")}
          className={view === "panel"
            // As many 330px cards as the row holds: Home has no width limit (PageBoard wide), so a wide screen adds columns.
            ? "grid grid-cols-[repeat(auto-fill,minmax(min(100%,330px),1fr))] gap-2.5"
            : "flex flex-col gap-1"}
        >
          {servers.map(renderServer)}
        </ul>
      )}
    </div>
  );
}
