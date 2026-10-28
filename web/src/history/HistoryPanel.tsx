import { Tag } from "../ui/Tag";
import { useCallback, useEffect, useState } from "react";
import { MoreDetails } from "../ui/MoreDetails";
import { toProblem } from "../api/guards";
import { useLanguage } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import type { Problem } from "../api/client";
import { configApi, type HistoryEntry, type PendingTransaction } from "../api/config";
import { Button, Notice } from "../ui/surface";
import { PageHeader, PageSection, SummaryStat, SummaryStats } from "../ui/page";
import { Icon } from "../ui/icons";
import { PanelState } from "../ui/PanelState";
import { formatDateTime } from "../ui/format";

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
  const { t, locale } = useLanguage();
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
    <div className={`flex flex-col gap-[22px] ${mobileTouchTargets}`}>
      <PageHeader title={t("history.pageTitle")} description={t("history.pageDescription")} />
      <SummaryStats>
        {([
          [t("history.metricChanges"), entries.length, false],
          [t("history.metricInterrupted"), pending.length, pending.length > 0],
          [t("history.metricRestorable"), restorableCount, false],
        ] as const).map(([label, value, attention]) => (
          <SummaryStat key={String(label)} label={String(label)} value={value} attention={Boolean(attention)} />
        ))}
      </SummaryStats>
      {problem === null ? null : (
        <Notice tone="danger">{t("history.requestRejected", { code: problem.code })}</Notice>
      )}
      {message === "" ? null : <Notice tone="success">{message}</Notice>}

      {pending.length === 0 ? null : (
        <section aria-labelledby="pending-heading" className="rounded-[12px] bg-notice">
          <header className="flex items-center gap-2 border-b border-notice-line px-4 py-3">
            <span className="flex size-[30px] items-center justify-center rounded-md bg-card text-notice-ink">
              <Icon name="history" className="size-4" />
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
        </section>
      )}

      <PageSection heading={t("history.completed")} headingId="history-heading" meta={String(entries.length)}>
        {entries.length === 0 ? (
          <PanelState tone="empty" title={t("history.empty")} />
        ) : (
          <ol className="divide-y divide-line">
            {entries.map((entry) => (
              <li key={entry.id} className="grid gap-4 py-3.5 md:grid-cols-[10rem_minmax(0,1fr)_auto] md:items-center">
                <time dateTime={entry.startedAt} title={entry.startedAt} className="text-xs text-ink-muted">{formatDateTime(entry.startedAt, locale)}</time>
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <p className="font-medium text-ink">{t(historyOperationLabelKey(entry.operation))}</p>
                    <Tag>{t(historyStatusLabelKey(entry.status))}</Tag>
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
        <p className="mt-3 text-xs leading-[18px] text-ink-faint">{t("history.backupRetention")}</p>
        <MoreDetails className="mt-1">{t("history.backupRetentionMore")}</MoreDetails>
      </PageSection>
    </div>
  );
}
