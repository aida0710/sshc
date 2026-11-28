import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
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
import { ConnectionCardBody, connectionCardButton, connectionCardClass, connectionCardGrid, connectionCardGroup } from "../connections/ConnectionCard";
import { hostMatchesQuery, normalizeHostQuery } from "../connections/hostSearch";
import { connectionAddress } from "../connections/connectionAddress";
import { revealOnHover } from "../ui/ActionMenu";
import { Spinner } from "../ui/Spinner";
import { formatDateTime } from "../ui/format";
import { resultCountText } from "../connections/resultCount";
import { mobileViewportQuery, useMediaQuery } from "../ui/useMediaQuery";
import { literalTextEntry } from "../ui/softKeyboard";
import { stuckToTopClass, useStuckToTop } from "../ui/useStuckToTop";

type QuickConnectBrowserProps = {
  // heading は、ページの見出し（と、その下の知らせ）。検索と同じ、一覧をスクロールしても上に残る所に置く。
  heading?: ReactNode;
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

// On a phone the groups are one line that scrolls sideways, so many of them leave the connections on the first
// screen. The negative margin lets the line run to the edges of the card.
const phoneGroupRow = "-mx-3 flex gap-2 overflow-x-auto px-3 [scrollbar-width:none]";
const phoneGroupChip = "max-w-[70%] shrink-0";

// 見出しから件数の行までは、一覧をスクロールしても上に残る。左右と上はページの板（PageBoard）の余白の外まで広げ、
// 張り付いたときに、下を流れるカードが余白に覗かないようにする。値は PageBoard の板の余白と同じ。
const pinnedTopEdges = {
  phone: "-mx-3 -mt-3.5 px-3 pt-3.5",
  wide: "-mx-8 -mt-[26px] px-8 pt-[26px]",
} as const;
// 張り付いている間だけ付ける下地と影。下を流れるカードが、透けてぼけて見えるようにする。地の色は、スマホでは
// ページの地（canvas）、PC では板の地（sidebar）に合わせる。
const pinnedTopGround = {
  phone: "bg-[color-mix(in_srgb,var(--ui-canvas)_84%,transparent)]",
  wide: "bg-[color-mix(in_srgb,var(--ui-sidebar)_84%,transparent)]",
} as const;
const pinnedTopGlass = "backdrop-blur-[10px] shadow-[0_4px_12px_rgb(0_0_0/0.08)]";

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
  heading,
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
  const pinnedTopRef = useRef<HTMLDivElement>(null);
  const pinnedTopStuck = useStuckToTop(pinnedTopRef);
  const layout = mobile ? "phone" : "wide";
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
    const lastConnectedAt = recentConnection === undefined ? null : formatDateTime(recentConnection.lastConnectedAt, locale);
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
        className={connectionCardClass(selected || opening)}
      >
        {/* busy-label-exempt: カードの名前（接続名）は変えず、その下に状態の行として回る印と「接続中…」を足している。 */}
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
          className={`${connectionCardButton} pr-12 disabled:cursor-wait hover-reveal:pr-3.5`}
        >
          <ConnectionCardBody
            icon={<OperatingSystemIcon os={server.os} colour={server.colour} look="accentTile" />}
            name={alias}
            lastConnected={lastConnectedAt === null || opening ? null : { at: lastConnectedAt, label: t("connection.lastConnected", { at: lastConnectedAt }) }}
            duplicateAlias={server.duplicateAlias}
            address={target}
            status={(
              // loading-line-exempt: 接続先のカードの状態の行（下の「接続中…」）を読み上げる枠。
              <span aria-live="polite">
                {opening ? (
                  // loading-line-exempt: 接続先のカードの状態の行。読み込みではなく、接続していることをカードの中で言う。
                  <span className="mt-1 flex items-center gap-1.5 text-2xs text-ink-muted">
                    <Spinner className="size-3" />
                    {t("home.opening")}
                  </span>
                ) : null}
              </span>
            )}
            // Where the "…" shows only under the pointer (hover-reveal), it appears over the group name; elsewhere the
            // card keeps room for it. A connection in no group shows no name: "no group" would only fill the room.
            end={server.group === "" ? null : (
              <span
                className={`${connectionCardGroup} transition-opacity duration-150 hover-reveal:group-hover/row:opacity-0 hover-reveal:group-focus-within/row:opacity-0`}
                title={server.group}
              >
                {server.group}
              </span>
            )}
          />
        </button>
        <div className={`pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 ${revealOnHover}`}>
          <ConnectionActions
            alias={alias}
            path={server.identity.path}
            busy={launching !== ""}
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

  // filters は、検索、表示の切り替え、グループ、件数の行。見出しと一緒に上に残る。
  const filters = (
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
              <button type="button" onClick={() => setGroupTrail([])} className="shrink-0 rounded-sm px-1 py-0.5 hover:bg-card hover:text-ink">
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
                      className={`truncate rounded-sm px-1 py-0.5 hover:bg-card hover:text-ink ${selected ? "font-medium text-ink" : ""}`}
                    >
                      {index.groupByName.get(name)?.label ?? name}
                    </button>
                  </span>
                );
              })}
            </nav>
          )}
          {visibleGroups.length === 0 ? null : (
            <div role="group" aria-label={t("home.groupFilter")} className={mobile ? phoneGroupRow : "flex flex-wrap gap-2"}>
              {visibleGroups.map((candidate) => {
                const childCount = index.visibleChildrenByParent.get(candidate.name)?.length ?? 0;
                return (
                  <button
                    key={candidate.name}
                    type="button"
                    title={candidate.name}
                    aria-label={t("home.openGroup", { name: candidate.name, count: candidate.descendantCount })}
                    onClick={() => setGroupTrail([...groupTrail, candidate.name])}
                    className={`flex min-h-9 items-center gap-2 rounded-md bg-well px-3 py-1.5 text-left transition-colors hover:bg-hover ${mobile ? phoneGroupChip : "min-w-0 max-w-full"}`}
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
        <h3 className="min-w-0 flex-1 text-xs font-normal text-ink-faint">{resultCountText(t, servers.length, index.servers.length)}</h3>
        {mobile ? viewSwitch : null}
      </div>
    </div>
  );

  return (
    <div className="flex flex-col">
      <div
        ref={pinnedTopRef}
        className={`${stuckToTopClass} z-[2] flex flex-col gap-[18px] pb-3 transition-[background-color,box-shadow] ${pinnedTopEdges[layout]} ${pinnedTopStuck ? `${pinnedTopGround[layout]} ${pinnedTopGlass}` : ""}`}
      >
        {heading}
        {filters}
      </div>

      {servers.length === 0 ? (
        <p className="rounded-inset bg-well p-4 text-sm text-ink-muted">
          {t("home.noMatches")}
        </p>
      ) : (
        <ul
          aria-label={t("home.connectionList")}
          className={view === "panel"
            // As many 330px cards as the row holds: Home has no width limit (PageBoard wide), so a wide screen adds columns.
            ? connectionCardGrid
            : "flex flex-col gap-1"}
        >
          {servers.map(renderServer)}
        </ul>
      )}
    </div>
  );
}
