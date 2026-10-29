import { useEffect, useRef, useState } from "react";
import { failureCode } from "../api/client";
import { configApi, type Overview } from "../api/config";
import { recentConnectionsApi, type RecentConnectionList } from "../api/recentConnections";
import { syncApi, type SyncStatus } from "../api/sync";
import { terminalSessionsApi } from "../api/terminalSessions";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { Button, Notice } from "../ui/surface";
import { PageHeader } from "../ui/page";
import { PageBoard } from "../ui/PageBoard";
import { QuickConnectBrowser } from "./QuickConnectBrowser";
import { attentionNoticesOf } from "../connections/noticeKinds";
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

  // Home counts only what asks the person to fix something, and says where to fix it: problems in the SSH
  // configuration files are shown in SSH Config, notices about connections and group folders in Connections,
  // and changes that stopped partway in History. A note that only explains a connection's values is not counted.
  const configProblems = overview === null
    ? 0
    : overview.diagnostics.filter((item) => item.severity === "error" || item.severity === "warning").length;
  const connectionNotices = overview === null ? 0 : attentionNoticesOf(overview.notices).length;
  const pendingRecovery = overview?.pending?.length ?? 0;
  const attention = configProblems + connectionNotices + pendingRecovery;

  return (
    <PageBoard labelledBy="home-heading" className="flex flex-col gap-[18px]">
      <PageHeader
        title={t("home.heading")}
        headingId="home-heading"
        meta={overview === null ? undefined : t("conn.count", { count: overview.hosts.filter((host) => host.identity.alias !== "").length })}
        actions={(
          <Button className="min-h-10 md:min-h-0" onClick={() => onNavigate("Connections")}>
            {t("home.manageConnections")}
          </Button>
        )}
      />

      {problem === "" ? null : <Notice tone="danger">{problem}</Notice>}
      {loading || overview === null || !listsFailed ? null : (
        <Notice actions={retryButton}>{t("home.listsLoadFailed")}</Notice>
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
        <section aria-label={t("home.attention")}>
          <Notice
            actions={(
              <>
                {configProblems === 0 ? null : <Button small onClick={() => onNavigate("Config")}>{t("home.openConfig")}</Button>}
                {connectionNotices === 0 ? null : <Button small onClick={() => onNavigate("Connections")}>{t("home.openConnectionNotices")}</Button>}
                {pendingRecovery === 0 ? null : <Button small onClick={() => onNavigate("History")}>{t("home.recoverChanges")}</Button>}
              </>
            )}
          >
            <Icon name="warning" className="size-[15px] shrink-0" />
            <div className="min-w-0">
              {configProblems === 0 ? null : <p>{t("home.configProblems", { count: configProblems })}</p>}
              {connectionNotices === 0 ? null : <p>{t("home.connectionNotices", { count: connectionNotices })}</p>}
              {pendingRecovery === 0 ? null : <p>{t("home.pendingRecovery", { count: pendingRecovery })}</p>}
            </div>
          </Notice>
        </section>
      )}

      <section aria-label={t("home.sync")} className="flex flex-wrap items-center gap-x-3 gap-y-2 border-t border-line pt-3.5 text-[12.5px] text-ink-muted">
        <Icon name="sync" className="size-3.5 shrink-0" />
        <p className="min-w-0 flex-1">
          {sync === null
            ? t("home.syncUnavailable")
            : !sync.configured
              ? t("home.syncNotConfigured")
              : sync.synced
                ? t("home.syncLast", { at: formatOptionalDateTime(sync.lastSyncedAt), count: sync.fileCount ?? 0 })
                : t("home.syncNever")}
        </p>
        <Button small onClick={() => onNavigate("Sync")}>{t("home.openSync")}</Button>
      </section>
    </PageBoard>
  );
}
