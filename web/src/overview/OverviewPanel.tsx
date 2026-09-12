import { useEffect, useRef, useState } from "react";
import { failureCode } from "../api/client";
import { configApi, type Overview } from "../api/config";
import { integrationsApi, type RecentConnectionList, type SyncStatus } from "../api/integrations";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { Button, Card, Notice } from "../ui/surface";
import { QuickConnectBrowser } from "./QuickConnectBrowser";
import { workspaceApi, type SavedWorkspace } from "../features/workspaces/api";
import { storedPaneCount } from "../features/workspaces/layout";
import { PanelState } from "../ui/PanelState";

export type OverviewDestination = "Connections" | "Config" | "Sync" | "History";

type OverviewPanelProps = {
  loadOverview?: () => Promise<Overview>;
  loadSync?: () => Promise<SyncStatus>;
  loadRecent?: () => Promise<RecentConnectionList>;
  loadWorkspaces?: () => Promise<SavedWorkspace[]>;
  launch?: (alias: string) => Promise<{ session: { id: string } }>;
  onNavigate: (destination: OverviewDestination) => void;
  onNavigateLocation: (location: string) => void;
  onConsoleOpened?: (id: string) => void;
  onOpenWorkspace?: (id: string) => void;
};

const loadDefaultOverview = () => configApi.overview();
const loadDefaultSync = () => integrationsApi.syncStatus();
const loadDefaultRecent = () => integrationsApi.recentConnections();
const loadDefaultWorkspaces = () => workspaceApi.list();
const launchDefault = (alias: string) => integrationsApi.openTerminalSession({ kind: "ssh", alias });
const informationalNoticeCodes = new Set(["group_empty"]);

export function OverviewPanel({
  loadOverview = loadDefaultOverview,
  loadSync = loadDefaultSync,
  loadRecent = loadDefaultRecent,
  loadWorkspaces = loadDefaultWorkspaces,
  launch = launchDefault,
  onNavigate,
  onNavigateLocation,
  onConsoleOpened,
  onOpenWorkspace = () => undefined,
}: OverviewPanelProps) {
  const t = useTranslate();
  const [overview, setOverview] = useState<Overview | null>(null);
  const [loading, setLoading] = useState(true);
  const [sync, setSync] = useState<SyncStatus | null>(null);
  const [recent, setRecent] = useState<RecentConnectionList["connections"]>([]);
  const [workspaces, setWorkspaces] = useState<SavedWorkspace[]>([]);
  const [launching, setLaunching] = useState("");
  const launchPending = useRef(false);
  const [problem, setProblem] = useState("");

  useEffect(() => {
    let active = true;
    void Promise.allSettled([loadOverview(), loadSync(), loadRecent(), loadWorkspaces()]).then(([workspace, remote, history, saved]) => {
      if (!active) return;
      if (workspace.status === "fulfilled") setOverview(workspace.value);
      else setProblem(t("home.loadFailed"));
      if (remote.status === "fulfilled") setSync(remote.value);
      if (history.status === "fulfilled") setRecent(history.value.connections);
      if (saved.status === "fulfilled") setWorkspaces(saved.value);
      setLoading(false);
    });
    return () => {
      active = false;
    };
  }, [loadOverview, loadSync, loadRecent, loadWorkspaces, t]);

  async function connect(alias: string) {
    if (launchPending.current) return;
    launchPending.current = true;
    setLaunching(alias);
    setProblem("");
    try {
      const opened = await launch(alias);
      onConsoleOpened?.(opened.session.id);
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
    <section aria-labelledby="home-heading" className="mx-auto flex w-full max-w-6xl flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line pb-3">
        <h2 id="home-heading" className="text-xl font-semibold tracking-tight">{t("home.heading")}</h2>
        <Button className="min-h-10 md:min-h-0" onClick={() => onNavigate("Connections")}>
          {t("home.manageConnections")}
        </Button>
      </div>

      {problem === "" ? null : <Notice tone="danger">{problem}</Notice>}

      <section aria-label={t("home.quickConnect")}>
        {loading ? (
          <PanelState tone="loading" title={t("home.loading")} />
        ) : overview === null ? null : (
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

      {workspaces.length === 0 ? null : (
        <Card as="section" aria-labelledby="saved-workspaces-heading" radius="sm">
          <div className="border-b border-line px-4 py-3 sm:px-5">
            <h3 id="saved-workspaces-heading" className="font-semibold">{t("home.savedWorkspaces")}</h3>
            <p className="mt-0.5 text-xs text-ink-muted">{t("home.savedWorkspacesHint")}</p>
          </div>
          <ul aria-label={t("home.savedWorkspaceList")} className="divide-y divide-line">
            {workspaces.map((workspace) => (
              <li key={workspace.id} className="flex min-w-0 flex-col gap-3 px-4 py-3 sm:flex-row sm:items-center sm:px-5">
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-semibold text-ink">{workspace.name}</p>
                  <p className="mt-0.5 truncate text-xs text-ink-muted">
                    {t("home.workspacePanes", { count: storedPaneCount(workspace.layout) })}
                    {" · "}
                    <time dateTime={workspace.updatedAt}>{t("home.workspaceUpdated", { at: formatConnectedAt(workspace.updatedAt) })}</time>
                  </p>
                </div>
                <Button
                  kind="primary"
                  className="min-h-10 w-full shrink-0 sm:w-auto md:min-h-0"
                  onClick={() => onOpenWorkspace(workspace.id)}
                >
                  {t("home.openWorkspace")}
                </Button>
              </li>
            ))}
          </ul>
        </Card>
      )}

      <section aria-label={t("home.sync")} className="flex flex-wrap items-center gap-x-3 gap-y-2 border-t border-line pt-3 text-xs text-ink-muted">
        <Icon name="sync" className="size-4 shrink-0" />
        <p className="min-w-0 flex-1">
          {sync === null
            ? t("home.syncUnavailable")
            : !sync.configured
              ? t("home.syncNotConfigured")
              : sync.synced
                ? t("home.syncLast", { at: sync.lastSyncedAt === undefined ? "—" : formatConnectedAt(sync.lastSyncedAt), count: sync.fileCount ?? 0 })
                : t("home.syncNever")}
        </p>
        <Button onClick={() => onNavigate("Sync")}>{t("home.openSync")}</Button>
      </section>
    </section>
  );
}

function formatConnectedAt(value: string): string {
  const connectedAt = new Date(value);
  if (Number.isNaN(connectedAt.valueOf())) return value;
  return new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(connectedAt);
}
