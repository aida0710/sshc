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
  // 開いたコンソールは接続画面で見る。開く場所と見る場所が違うので、
  // その受け渡しだけをシェルが仲介する。
  onConsoleOpened?: (id: string) => void;
};

const loadDefaultOverview = () => configApi.overview();
const loadDefaultSync = () => integrationsApi.syncStatus();
const launchDefault = (alias: string) => integrationsApi.openTerminalSession({ kind: "ssh", alias });
const informationalNoticeCodes = new Set(["group_empty"]);

// Home は管理画面の要約ではなく接続の入口である。画面を開いただけでは
// DNS、TCP、ssh のどれにも触れない。接続はこのアプリケーションの中で開き、
// 開いたコンソールは接続画面へ持っていく。
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
      // **開いた端末を見せる。** onConsoleOpened がターミナルの面へ連れて行く
      // ので、そのあとに別の面を指すと、繋いだ本人が接続一覧へ放り出される
      // ——押した理由がそこには無い。
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

  return (
    <section aria-labelledby="home-heading" className="mx-auto flex w-full max-w-6xl flex-col gap-6">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h2 id="home-heading" className="text-xl font-semibold">{t("home.heading")}</h2>
          <p className="mt-1 text-sm text-ink-muted">{t("home.intro")}</p>
        </div>
        <Button onClick={() => onNavigate("Connections")}>
          {t("home.manageConnections")}
        </Button>
      </div>

      {problem === "" ? null : <Notice tone="danger">{problem}</Notice>}

      <div className="grid gap-3 sm:grid-cols-3">
        <Summary label={t("home.connections")} value={overview === null ? "—" : connectionCount} />
        <Summary label={t("home.groups")} value={overview === null ? "—" : overview.groups.length} />
        <Summary label={t("home.attention")} value={overview === null ? "—" : attention} attention={attention > 0} />
      </div>

      <section aria-labelledby="quick-connect-heading" className="flex flex-col gap-3">
        <div>
          <h3 id="quick-connect-heading" className="font-medium">{t("home.quickConnect")}</h3>
          <p className={hintText}>{t("home.quickConnectHint")}</p>
        </div>

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
      </section>

      <div className="grid gap-3 md:grid-cols-2">
        <section className="rounded-xl border border-line bg-card p-4">
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
              <Button onClick={() => onNavigate("Config")}>{t("home.openConfig")}</Button>
            )}
            {recoveryAttention === 0 ? null : (
              <Button onClick={() => onNavigate("History")}>{t("home.recoverChanges")}</Button>
            )}
          </div>
        </section>
        <section className="rounded-xl border border-line bg-card p-4">
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
          <Button className="mt-3" onClick={() => onNavigate("Sync")}>{t("home.openSync")}</Button>
        </section>
      </div>
    </section>
  );
}

function Summary({ label, value, attention = false }: { label: string; value: string | number; attention?: boolean }) {
  return (
    <div className={`rounded-xl border bg-card p-4 ${attention ? "border-notice-line" : "border-line"}`}>
      <p className="text-xs font-medium uppercase tracking-wide text-ink-muted">{label}</p>
      <p className={`mt-1 text-2xl font-semibold ${attention ? "text-notice-ink" : "text-ink"}`}>{value}</p>
    </div>
  );
}
