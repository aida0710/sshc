import { useEffect, useRef, useState } from "react";
import { failureCode } from "../api/client";
import { configApi, type Overview } from "../api/config";
import { recentConnectionsApi, type RecentConnectionList } from "../api/recentConnections";
import { syncApi, type SyncStatus } from "../api/sync";
import { terminalSessionsApi } from "../api/terminalSessions";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { Button, Notice } from "../ui/surface";
import { SectionBar } from "../ui/SectionBar";
import { QuickConnectBrowser } from "./QuickConnectBrowser";
import { PanelState } from "../ui/PanelState";
import { formatOptionalDateTime } from "../ui/format";

export type OverviewDestination = "Connections" | "Config" | "Sync" | "History";

type OverviewPanelProps = {
  loadOverview?: () => Promise<Overview>;
  loadSync?: () => Promise<SyncStatus>;
  loadRecent?: () => Promise<RecentConnectionList>;
  launch?: (alias: string) => Promise<{ session: { id: string } }>;
  onNavigate: (destination: OverviewDestination) => void;
  onNavigateLocation: (location: string) => void;
  onSessionOpened?: (id: string) => void;
};

const loadDefaultOverview = () => configApi.overview();
const loadDefaultSync = () => syncApi.syncStatus();
const loadDefaultRecent = () => recentConnectionsApi.recentConnections();
const launchDefault = (alias: string) => terminalSessionsApi.openTerminalSession({ kind: "ssh", alias });
const informationalNoticeCodes = new Set(["group_empty"]);

export function OverviewPanel({
  loadOverview = loadDefaultOverview,
  loadSync = loadDefaultSync,
  loadRecent = loadDefaultRecent,
  launch = launchDefault,
  onNavigate,
  onNavigateLocation,
  onSessionOpened,
}: OverviewPanelProps) {
  const t = useTranslate();
  const [overview, setOverview] = useState<Overview | null>(null);
  const [loading, setLoading] = useState(true);
  const [sync, setSync] = useState<SyncStatus | null>(null);
  const [recent, setRecent] = useState<RecentConnectionList["connections"]>([]);
  const [launching, setLaunching] = useState("");
  const launchPending = useRef(false);
  const [problem, setProblem] = useState("");
  // 読めなかったことを「無い」と見分けられるように、履歴の失敗は空の一覧とは
  // 別に持つ。SSH設定を読めなかったときは overview を null にする。
  const [listsFailed, setListsFailed] = useState(false);
  const [loadAttempt, setLoadAttempt] = useState(0);

  useEffect(() => {
    let active = true;
    void Promise.allSettled([loadOverview(), loadSync(), loadRecent()]).then(([workspace, remote, history]) => {
      if (!active) return;
      setOverview(workspace.status === "fulfilled" ? workspace.value : null);
      if (remote.status === "fulfilled") setSync(remote.value);
      if (history.status === "fulfilled") setRecent(history.value.connections);
      setListsFailed(history.status === "rejected");
      setLoading(false);
    });
    return () => {
      active = false;
    };
  }, [loadOverview, loadSync, loadRecent, loadAttempt]);

  function reload() {
    setLoading(true);
    setLoadAttempt((attempt) => attempt + 1);
  }

  const retryButton = <Button onClick={reload}>{t("shell.bootstrapRetry")}</Button>;

  async function connect(alias: string) {
    if (launchPending.current) return;
    launchPending.current = true;
    setLaunching(alias);
    setProblem("");
    try {
      const opened = await launch(alias);
      onSessionOpened?.(opened.session.id);
    } catch (error) {
      setProblem(
        failureCode(error) === "terminal_session_limit"
          ? t("terminal.limitRefused")
          : t("terminal.openFailed"),
      );
    } finally {
      launchPending.current = false;
      setLaunching("");
    }
  }

  const configurationAttention = overview === null
    ? 0
    : overview.diagnostics.filter((item) => item.severity === "error" || item.severity === "warning").length +
      overview.notices.filter((item) => !informationalNoticeCodes.has(item.code)).length;
  const recoveryAttention = overview?.pending?.length ?? 0;
  const attention = configurationAttention + recoveryAttention;

  return (
    <div className="flex h-full min-h-0 flex-col bg-canvas">
    <SectionBar
      title={t("home.heading")}
      {...(overview === null ? {} : { meta: t("conn.count", { count: overview.hosts.filter((host) => host.identity.alias !== "").length }) })}
      actions={(
        <Button className="min-h-10 md:min-h-0" onClick={() => onNavigate("Connections")}>
          {t("home.manageConnections")}
        </Button>
      )}
    />
    <section aria-label={t("home.heading")} className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto p-4 md:p-5">

      {problem === "" ? null : <Notice tone="danger">{problem}</Notice>}
      {loading || overview === null || !listsFailed ? null : (
        <Notice>
          <span className="grow">{t("home.listsLoadFailed")}</span>
          {retryButton}
        </Notice>
      )}

      <section aria-label={t("home.quickConnect")}>
        {loading ? (
          <PanelState tone="loading" title={t("home.loading")} />
        ) : overview === null ? (
          <PanelState tone="failed" title={t("home.loadFailed")} action={retryButton} />
        ) : (
          <QuickConnectBrowser
            overview={overview}
            recent={recent}
            launching={launching}
            onConnect={(alias) => void connect(alias)}
            onOpenSettings={onNavigateLocation}
          />
        )}
      </section>

      {attention === 0 ? null : (
        <section aria-label={t("home.attention")} className="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-lg border border-notice-line bg-notice px-4 py-3">
          <div className="flex min-w-0 flex-1 items-center gap-2 text-sm text-notice-ink">
            <span aria-hidden="true" className="size-2 shrink-0 rounded-full bg-notice-ink" />
            <p>{t("home.workspaceAttention", { count: attention })}</p>
          </div>
          {configurationAttention === 0 ? null : (
            <Button onClick={() => onNavigate("Config")}>{t("home.openConfig")}</Button>
          )}
          {recoveryAttention === 0 ? null : (
            <Button onClick={() => onNavigate("History")}>{t("home.recoverChanges")}</Button>
          )}
        </section>
      )}

      <section aria-label={t("home.sync")} className="flex flex-wrap items-center gap-x-3 gap-y-2 border-t border-line pt-3 text-xs text-ink-muted">
        <Icon name="sync" className="size-4 shrink-0" />
        <p className="min-w-0 flex-1">
          {sync === null
            ? t("home.syncUnavailable")
            : !sync.configured
              ? t("home.syncNotConfigured")
              : sync.synced
                ? t("home.syncLast", { at: formatOptionalDateTime(sync.lastSyncedAt), count: sync.fileCount ?? 0 })
                : t("home.syncNever")}
        </p>
        <Button onClick={() => onNavigate("Sync")}>{t("home.openSync")}</Button>
      </section>
    </section>
    </div>
  );
}
