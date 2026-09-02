import { useCallback, useEffect, useState } from "react";
import { toProblem } from "../api/guards";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import type { Problem } from "../api/client";
import { configApi, type HistoryEntry, type PendingTransaction } from "../api/config";
import { Button, Card, Notice } from "../ui/surface";
import { MetricCard, MetricGrid, PageHeader } from "../ui/page";
import { Icon } from "../ui/icons";
import { PanelState } from "../ui/PanelState";

const mobileTouchTargets = "[&_button]:min-h-10 md:[&_button]:min-h-0";

export function historyOperationLabelKey(operation: string): MessageKey {
  if (operation.startsWith("connection.")) return "history.operation.connection";
  if (operation.startsWith("key.")) return "history.operation.key";
  if (operation.startsWith("terminal.")) return "history.operation.terminal";
  if (operation.startsWith("engine.")) return "history.operation.engine";
  if (operation.startsWith("secret.")) return "history.operation.vault";
  if (operation.startsWith("sync.")) return "history.operation.sync";
  if (operation.startsWith("config.") || operation.startsWith("directory.") || operation.startsWith("group.")) {
    return "history.operation.configuration";
  }
  return "history.operation.other";
}

export function historyStatusLabelKey(status: string): MessageKey {
  switch (status) {
    case "staging": return "history.status.staging";
    case "staged": return "history.status.staged";
    case "applied": return "history.status.applied";
    case "completed": return "history.status.completed";
    case "rolled_back": return "history.status.rolledBack";
    default: return "history.status.unknown";
  }
}

export function HistoryPanel() {
  const t = useTranslate();
  const [entries, setEntries] = useState<HistoryEntry[] | null>(null);
  const [pending, setPending] = useState<PendingTransaction[]>([]);
  const [problem, setProblem] = useState<Problem | null>(null);
  const [message, setMessage] = useState("");

  const reload = useCallback(async () => {
    const [historyResult, overviewResult] = await Promise.allSettled([
      configApi.history(),
      configApi.overview(),
    ]);
    let nextProblem: Problem | null = null;

    if (historyResult.status === "fulfilled") {
      setEntries(historyResult.value);
    } else {
      setEntries((current) => current ?? []);
      nextProblem = toProblem(historyResult.reason);
    }

    if (overviewResult.status === "fulfilled") {
      setPending(overviewResult.value.pending ?? []);
    } else if (nextProblem === null) {
      nextProblem = toProblem(overviewResult.reason);
    }

    setProblem(nextProblem);
  }, []);

  useEffect(() => {
    void reload();
  }, [reload]);

  async function restore(transactionId: string, path: string) {
    try {
      const result = await configApi.restore(transactionId, path);
      setMessage(t("history.restored", { path, id: result.transactionId }));
      setProblem(null);
      await reload();
    } catch (error) {
      setProblem(toProblem(error));
    }
  }

  async function recover(transactionId: string, action: "complete" | "rollback") {
    try {
      await configApi.recover(transactionId, action);
      setMessage(t(action === "complete" ? "history.completedTransaction" : "history.rolledBack"));
      setProblem(null);
      await reload();
    } catch (error) {
      setProblem(toProblem(error));
    }
  }

  if (entries === null) {
    return <PanelState tone="loading" title={t("history.loading")} />;
  }

  const restorableCount = entries.reduce((count, entry) => count + (entry.restorable?.length ?? 0), 0);

  return (
    <div className={`mx-auto flex w-full max-w-6xl flex-col gap-6 ${mobileTouchTargets}`}>
      <PageHeader title={t("history.pageTitle")} description={t("history.pageDescription")} />
      <MetricGrid className="sm:grid-cols-3 lg:grid-cols-3">
        {([
          [t("history.metricChanges"), entries.length, false],
          [t("history.metricInterrupted"), pending.length, pending.length > 0],
          [t("history.metricRestorable"), restorableCount, false],
        ] as const).map(([label, value, attention]) => (
          <MetricCard key={String(label)} label={String(label)} value={value} compact attention={Boolean(attention)} />
        ))}
      </MetricGrid>
      {problem === null ? null : (
        <Notice tone="danger">{t("history.requestRejected", { code: problem.code })}</Notice>
      )}
      {message === "" ? null : <p role="status" className="text-sm text-live">{message}</p>}

      {pending.length === 0 ? null : (
        <Card as="section" aria-labelledby="pending-heading" radius="md" tone="notice">
          <header className="flex items-center gap-2 border-b border-notice-line px-4 py-3">
            <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-card text-notice-ink">
              <Icon name="history" className="h-4 w-4" />
            </span>
            <h3 id="pending-heading" className="text-sm font-semibold text-notice-ink">{t("history.interrupted")}</h3>
          </header>
          <div className="divide-y divide-notice-line">
            {pending.map((item) => (
              <article key={item.id} className="grid gap-4 px-4 py-4 md:grid-cols-[minmax(0,1fr)_auto] md:items-center">
                <div className="min-w-0">
                  <p className="text-sm font-medium text-ink">{t(historyOperationLabelKey(item.operation))}</p>
                  <p className="mt-1 text-xs text-ink-muted">
                    {t("history.interruptedDetail", {
                      operation: t(historyOperationLabelKey(item.operation)),
                      startedAt: item.startedAt,
                      committed: item.committed,
                      total: item.paths.length,
                    })}
                  </p>
                  <p className="mt-2 truncate font-mono text-xs text-ink-muted">{item.paths.join(", ")}</p>
                </div>
                <div className="flex gap-2">
                  <Button
                    disabled={!item.canComplete}
                    onClick={() => void recover(item.id, "complete")}
                  >
                    {t("history.complete")}
                  </Button>
                  <Button onClick={() => void recover(item.id, "rollback")}>{t("history.rollBack")}</Button>
                </div>
              </article>
            ))}
          </div>
        </Card>
      )}

      <Card as="section" aria-labelledby="history-heading" radius="md">
        <header className="flex items-center justify-between gap-3 border-b border-line bg-toolbar px-4 py-3">
          <div className="flex items-center gap-2">
            <Icon name="history" className="h-4 w-4 text-ink-muted" />
            <h3 id="history-heading" className="text-sm font-semibold text-ink">{t("history.completed")}</h3>
          </div>
          <span className="rounded-md bg-surface px-2 py-0.5 font-mono text-xs text-ink-muted">{entries.length}</span>
        </header>
        {entries.length === 0 ? (
          <PanelState tone="empty" title={t("history.empty")} />
        ) : (
          <ol className="divide-y divide-line">
            {entries.map((entry) => (
              <li key={entry.id} className="grid gap-4 px-4 py-4 md:grid-cols-[10rem_minmax(0,1fr)_auto] md:items-center">
                <time dateTime={entry.startedAt} className="font-mono text-xs text-ink-muted">{entry.startedAt}</time>
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <p className="font-medium text-ink">{t(historyOperationLabelKey(entry.operation))}</p>
                    <span className="rounded-md bg-select-fill px-2 py-0.5 text-[11px] text-ink-muted">{t(historyStatusLabelKey(entry.status))}</span>
                  </div>
                  <p className="mt-1 truncate font-mono text-xs text-ink-faint">{entry.paths.join(", ")}</p>
                </div>
                <div className="flex flex-wrap gap-2 md:justify-end">
                  {(entry.restorable ?? []).map((path) => (
                    <Button key={path} onClick={() => void restore(entry.id, path)}>
                      {t("history.restorePath", { path })}
                    </Button>
                  ))}
                </div>
              </li>
            ))}
          </ol>
        )}
        <p className="border-t border-line bg-toolbar px-4 py-3 text-xs text-ink-faint">{t("history.backupsKept")}</p>
      </Card>
    </div>
  );
}
