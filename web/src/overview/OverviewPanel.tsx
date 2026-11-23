import { useEffect, useRef, useState } from "react";
import { configApi, type Overview } from "../api/config";
import { recentConnectionsApi, type RecentConnectionList } from "../api/recentConnections";
import { syncApi, type SyncStatus } from "../api/sync";
import { useLanguage } from "../i18n/context";
import { Icon } from "../ui/icons";
import { Button, Notice } from "../ui/surface";
import { PageHeader } from "../ui/page";
import { PageBoard } from "../ui/PageBoard";
import { QuickConnectBrowser } from "./QuickConnectBrowser";
import { presetInUse, useSettings } from "../settings/settingsStore";
import { emptyTerminalArtOf } from "./dashboard/emptyTerminalArt";
import { HomeBackdrop } from "./dashboard/HomeBackdrop";
import { HomeMachineSection } from "./dashboard/HomeMachineSection";
import { useTerminalOverview } from "./dashboard/useTerminalOverview";
import { ConnectionStartTiles } from "../connections/ConnectionStartTiles";
import { attentionNoticesOf } from "../connections/noticeKinds";
import { PanelState } from "../ui/PanelState";
import { formatOptionalDateTime } from "../ui/format";

export type OverviewDestination = "Connections" | "Config" | "Sync" | "History";

type OverviewPanelProps = {
  // runTarget is the engine new sessions open on; null is this machine. The foot of Home shows that machine.
  runTarget: string | null;
  loadOverview?: () => Promise<Overview>;
  loadSync?: () => Promise<SyncStatus>;
  loadRecent?: () => Promise<RecentConnectionList>;
  // onConnect opens a session to the connection on the engine new sessions open on, and shows it. It settles once
  // the engine has answered; a failure is said where the terminal's other failures are.
  onConnect: (alias: string) => Promise<void>;
  // sessionProblem is why the last session did not open, or empty. The sidebar says it too, and a phone hides the
  // sidebar, so Home says it there.
  sessionProblem?: string;
  onNavigate: (destination: OverviewDestination) => void;
  onNavigateLocation: (location: string) => void;
  onAddConnection: () => void;
  onOpenLocalShell: () => void;
};

const loadDefaultOverview = () => configApi.overview();
const loadDefaultSync = () => syncApi.syncStatus();
const loadDefaultRecent = () => recentConnectionsApi.recentConnections();

export function OverviewPanel({
  runTarget,
  loadOverview = loadDefaultOverview,
  loadSync = loadDefaultSync,
  loadRecent = loadDefaultRecent,
  onConnect,
  sessionProblem = "",
  onNavigate,
  onNavigateLocation,
  onAddConnection,
  onOpenLocalShell,
}: OverviewPanelProps) {
  const { t, locale } = useLanguage();
  const machineOverview = useTerminalOverview(runTarget);
  const art = emptyTerminalArtOf(presetInUse(useSettings().document));
  const [overview, setOverview] = useState<Overview | null>(null);
  const [loading, setLoading] = useState(true);
  const [sync, setSync] = useState<SyncStatus | null>(null);
  const [recent, setRecent] = useState<RecentConnectionList["connections"]>([]);
  const [launching, setLaunching] = useState("");
  const launchPending = useRef(false);
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
    try {
      await onConnect(alias);
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
  const connectionCount = overview === null ? 0 : overview.hosts.filter((host) => host.identity.alias !== "").length;
  // With no connection yet there is nothing to search, count or manage, and the tiles lead to Sync.
  const noConnections = overview !== null && connectionCount === 0;

  return (
    <PageBoard wide stretch={noConnections} labelledBy="home-heading" className="relative isolate flex flex-col gap-[18px]">
      {art === "none" ? null : <HomeBackdrop art={art} history={machineOverview.history} />}
      <PageHeader
        title={t("home.heading")}
        headingId="home-heading"
        actions={noConnections ? undefined : (
          <Button className="min-h-10 md:min-h-0" onClick={() => onNavigate("Connections")}>
            {t("home.manageConnections")}
          </Button>
        )}
      />

      {sessionProblem === "" ? null : (
        <div className="hidden mobile-layout:block"><Notice tone="danger">{sessionProblem}</Notice></div>
      )}
      {loading || overview === null || !listsFailed ? null : (
        <Notice actions={retryButton}>{t("home.listsLoadFailed")}</Notice>
      )}

      <section aria-label={t("home.quickConnect")} className={noConnections ? "flex flex-1 flex-col" : undefined}>
        {loading ? (
          <PanelState tone="loading" title={t("home.loading")} />
        ) : overview === null ? (
          <PanelState tone="failed" title={t("home.loadFailed")} action={retryButton} />
        ) : noConnections ? (
          <ConnectionStartTiles
            onAddConnection={onAddConnection}
            onOpenConfig={() => onNavigate("Config")}
            onOpenSync={() => onNavigate("Sync")}
            onOpenLocalShell={onOpenLocalShell}
          />
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

      {/* Before the answer arrives, no status would read as a status that cannot be read. */}
      {noConnections || loading ? null : (
        <section aria-label={t("home.sync")} className="flex flex-wrap items-center gap-x-3 gap-y-2 border-t border-line pt-3.5 text-xs text-ink-muted">
          <Icon name="sync" className="size-3.5 shrink-0" />
          <p className="min-w-0 flex-1">
            {sync === null
              ? t("home.syncUnavailable")
              : !sync.configured
                ? t("home.syncNotConfigured")
                : sync.synced
                  ? t("home.syncLast", { at: formatOptionalDateTime(sync.lastSyncedAt, locale), count: sync.fileCount ?? 0 })
                  : t("home.syncNever")}
          </p>
          <Button small onClick={() => onNavigate("Sync")}>{t("home.openSync")}</Button>
        </section>
      )}

      <HomeMachineSection overview={machineOverview} runTarget={runTarget} />
    </PageBoard>
  );
}
