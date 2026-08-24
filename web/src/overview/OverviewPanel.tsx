import { useEffect, useState } from "react";
import { failureCode } from "../api/client";
import { configApi, type Overview } from "../api/config";
import { integrationsApi, type SyncStatus } from "../api/integrations";
import { useTranslate } from "../i18n/context";
import { hintText } from "../ui/form";
import { Button, Notice } from "../ui/surface";
import { QuickConnectBrowser } from "./QuickConnectBrowser";

export type OverviewDestination = "Connections" | "Config" | "Sync" | "History";

type OverviewPanelProps = {
  loadOverview?: () => Promise<Overview>;
  loadSync?: () => Promise<SyncStatus>;
  launch?: (alias: string) => Promise<{ session: { id: string } }>;
  onNavigate: (destination: OverviewDestination) => void;
  onNavigateLocation: (location: string) => void;
  onConsoleOpened?: (id: string) => void;
};

const loadDefaultOverview = () => configApi.overview();
const loadDefaultSync = () => integrationsApi.syncStatus();
const launchDefault = (alias: string) => integrationsApi.openTerminalSession({ kind: "ssh", alias });
const informationalNoticeCodes = new Set(["group_empty"]);

export function OverviewPanel({
  loadOverview = loadDefaultOverview,
  loadSync = loadDefaultSync,
  launch = launchDefault,
  onNavigate,
  onNavigateLocation,
  onConsoleOpened,
}: OverviewPanelProps) {
  const t = useTranslate();
  const [overview, setOverview] = useState<Overview | null>(null);
  const [loading, setLoading] = useState(true);
  const [sync, setSync] = useState<SyncStatus | null>(null);
  const [launching, setLaunching] = useState("");
  const [problem, setProblem] = useState("");

  useEffect(() => {
    let active = true;
    void Promise.allSettled([loadOverview(), loadSync()]).then(([workspace, remote]) => {
      if (!active) return;
      if (workspace.status === "fulfilled") setOverview(workspace.value);
      else setProblem(t("home.loadFailed"));
      if (remote.status === "fulfilled") setSync(remote.value);
      setLoading(false);
    });
    return () => {
      active = false;
    };
  }, [loadOverview, loadSync, t]);

  async function connect(alias: string) {
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
      setLaunching("");
    }
  }

  const configurationAttention = overview === null
    ? 0
    : overview.diagnostics.filter((item) => item.severity === "error" || item.severity === "warning").length +
      overview.notices.filter((item) => !informationalNoticeCodes.has(item.code)).length;
  const recoveryAttention = overview?.pending?.length ?? 0;
  const attention = configurationAttention + recoveryAttention;
  const connectionCount = overview?.hosts.filter((host) => host.identity.alias !== "").length ?? 0;
  const favouriteCount = overview?.metadata.hosts?.filter((host) => host.favourite === true).length ?? 0;

  return (
    <section aria-labelledby="home-heading" className="mx-auto flex w-full max-w-6xl flex-col gap-5">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h2 id="home-heading" className="text-2xl font-semibold tracking-tight">{t("home.heading")}</h2>
          <p className="mt-1 text-sm text-ink-muted">{t("home.intro")}</p>
        </div>
        <Button className="min-h-10 md:min-h-0" onClick={() => onNavigate("Connections")}>
          {t("home.manageConnections")}
        </Button>
      </div>

      {problem === "" ? null : <Notice tone="danger">{problem}</Notice>}

      <section
        aria-labelledby="quick-connect-heading"
        className="overflow-hidden rounded-2xl border border-line bg-card shadow-[0_18px_50px_-38px_rgba(15,23,42,0.7)]"
      >
        <div className="sshc-connect-hero relative overflow-hidden px-5 py-5 text-white sm:px-6">
          <div aria-hidden="true" className="absolute -right-10 -top-16 size-44 rounded-full border border-white/10" />
          <div aria-hidden="true" className="absolute -right-2 -top-4 size-24 rounded-full border border-white/10" />
          <div className="relative flex flex-wrap items-start justify-between gap-4">
            <div className="flex min-w-0 items-start gap-3">
              <span
                aria-hidden="true"
                className="mt-0.5 grid size-9 shrink-0 place-items-center rounded-lg border border-white/15 bg-white/10 font-mono text-sm text-connect-mark"
              >
                &gt;_
              </span>
              <div>
                <h3 id="quick-connect-heading" className="text-base font-semibold tracking-tight">
                  {t("home.quickConnect")}
                </h3>
                <p className="mt-1 text-sm text-white/70">{t("home.quickConnectHint")}</p>
              </div>
            </div>
            <p className="rounded-full border border-white/15 bg-white/10 px-3 py-1 text-xs text-white/80">
              <span aria-hidden="true" className="mr-1.5 text-connect-star">★</span>
              {t("home.favourite")} · {overview === null ? "—" : favouriteCount}
            </p>
          </div>
        </div>

        <div className="p-4 sm:p-5">
          {loading ? (
            <p aria-live="polite" className={hintText}>{t("home.loading")}</p>
          ) : overview === null ? null : (
            <QuickConnectBrowser
              overview={overview}
              launching={launching}
              onConnect={(alias) => void connect(alias)}
              onOpenSettings={onNavigateLocation}
            />
          )}
        </div>
      </section>

      <dl
        role="group"
        aria-label={`${t("home.connections")}, ${t("home.groups")}, ${t("home.attention")}`}
        className="grid overflow-hidden rounded-xl border border-line bg-toolbar sm:grid-cols-3"
      >
        <Summary label={t("home.connections")} value={overview === null ? "—" : connectionCount} />
        <Summary label={t("home.groups")} value={overview === null ? "—" : overview.groups.length} />
        <Summary label={t("home.attention")} value={overview === null ? "—" : attention} attention={attention > 0} />
      </dl>

      <div className="grid gap-3 md:grid-cols-2">
        <section className="sshc-card rounded-xl bg-card p-4">
          <div className="flex items-start gap-3">
            <span
              aria-hidden="true"
              className={`mt-1 size-2 shrink-0 rounded-full ${attention > 0 ? "bg-notice-ink" : "bg-live"}`}
            />
            <div className="min-w-0 flex-1">
              <h3 className="font-medium">{t("home.workspace")}</h3>
              <p className="mt-1 text-sm text-ink-muted">
                {overview === null
                  ? t("home.workspaceUnavailable")
                  : attention === 0
                    ? t("home.workspaceClean")
                    : t("home.workspaceAttention", { count: attention })}
              </p>
              <div className="mt-3 flex flex-wrap gap-2">
                {configurationAttention === 0 ? null : (
                  <Button className="min-h-10 md:min-h-0" onClick={() => onNavigate("Config")}>{t("home.openConfig")}</Button>
                )}
                {recoveryAttention === 0 ? null : (
                  <Button className="min-h-10 md:min-h-0" onClick={() => onNavigate("History")}>{t("home.recoverChanges")}</Button>
                )}
              </div>
            </div>
          </div>
        </section>
        <section className="sshc-card rounded-xl bg-card p-4">
          <div className="flex items-start gap-3">
            <span aria-hidden="true" className="mt-1 font-mono text-xs text-accent">↕</span>
            <div className="min-w-0 flex-1">
              <h3 className="font-medium">{t("home.sync")}</h3>
              <p className="mt-1 text-sm text-ink-muted">
                {sync === null
                  ? t("home.syncUnavailable")
                  : !sync.configured
                    ? t("home.syncNotConfigured")
                    : sync.synced
                      ? t("home.syncLast", { at: sync.lastSyncedAt ?? "—", count: sync.fileCount ?? 0 })
                      : t("home.syncNever")}
              </p>
              <Button className="mt-3 min-h-10 md:min-h-0" onClick={() => onNavigate("Sync")}>{t("home.openSync")}</Button>
            </div>
          </div>
        </section>
      </div>
    </section>
  );
}

function Summary({ label, value, attention = false }: { label: string; value: string | number; attention?: boolean }) {
  return (
    <div className="flex items-center justify-between gap-4 border-t border-line px-4 py-3 first:border-t-0 sm:border-l sm:border-t-0 sm:first:border-l-0">
      <dt className="text-xs font-medium uppercase tracking-wide text-ink-muted">{label}</dt>
      <dd className={`font-mono text-lg font-semibold tabular-nums ${attention ? "text-notice-ink" : "text-ink"}`}>
        {value}
      </dd>
    </div>
  );
}
