import { Tag } from "../ui/Tag";
import { useCallback, useEffect, useState } from "react";
import { MoreDetails } from "../ui/MoreDetails";
import { toProblem } from "../api/guards";
import { useLanguage } from "../i18n/context";
import type { Problem } from "../api/client";
import { configApi, type HistoryEntry, type PendingTransaction } from "../api/config";
import { Button, Notice } from "../ui/surface";
import { PageHeader, PageSection, PageStateFrame, SummaryStat, SummaryStats } from "../ui/page";
import { Icon } from "../ui/icons";
import { PanelState } from "../ui/PanelState";
import { formatDateTime } from "../ui/format";
import { saveProblemMessage } from "../ui/saveProblemMessage";
import { historyOperationLabelKey, historyStatusLabelKey, restorableFileName, shownHistoryPaths } from "./historyLabels";
import { PendingChangeDialog } from "./PendingChangeDialog";

const mobileTouchTargets = "[&_button]:min-h-10 md:[&_button]:min-h-0";

// A change's columns: when, what, and the buttons that restore its files. The buttons' column takes what they need up
// to 40% and no more. A file's path has no place to break, so left to itself it would take the whole row and squeeze
// the name of the change to one letter a line.
const entryColumns = "grid grid-cols-[minmax(0,1fr)] gap-4 py-3.5 md:grid-cols-[10rem_minmax(0,1fr)_fit-content(40%)] md:items-center";

// The name of the file is what the button is for, so a long one wraps rather than being cut off.
const restoreButtonWraps = "min-w-0 whitespace-normal! text-left [overflow-wrap:anywhere]";

export function HistoryPanel() {
  const { t, locale } = useLanguage();
  const [entries, setEntries] = useState<HistoryEntry[] | null>(null);
  const [pending, setPending] = useState<PendingTransaction[]>([]);
  // What the last restore or recovery was refused for. Failing to read the history is a different thing to say: nothing
  // was attempted, so "nothing was written" would not be an answer.
  const [problem, setProblem] = useState<Problem | null>(null);
  const [readFailed, setReadFailed] = useState(false);
  const [message, setMessage] = useState("");
  const [inspected, setInspected] = useState<PendingTransaction | null>(null);

  const reload = useCallback(async () => {
    const [historyResult, overviewResult] = await Promise.allSettled([
      configApi.history(),
      configApi.overview(),
    ]);
    if (historyResult.status === "fulfilled") {
      setEntries(historyResult.value);
    } else {
      setEntries((current) => current ?? []);
    }

    if (overviewResult.status === "fulfilled") {
      setPending(overviewResult.value.pending ?? []);
    }

    setReadFailed(historyResult.status === "rejected" || overviewResult.status === "rejected");
  }, []);

  useEffect(() => {
    void reload();
  }, [reload]);

  async function restore(transactionId: string, path: string) {
    try {
      await configApi.restore(transactionId, path);
      setMessage(t("history.restored", { path: restorableFileName(path, t) }));
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
    return (
      <PageStateFrame title={t("history.pageTitle")} description={t("history.pageDescription")}>
        <PanelState tone="loading" title={t("history.loading")} />
      </PageStateFrame>
    );
  }

  const restorableCount = entries.reduce((count, entry) => count + (entry.restorable?.length ?? 0), 0);

  return (
    <div className={`flex flex-col gap-[22px] ${mobileTouchTargets}`}>
      <PageHeader title={t("history.pageTitle")} description={t("history.pageDescription")} />
      <SummaryStats>
        <SummaryStat label={t("history.metricChanges")} value={entries.length} />
        {pending.length === 0 ? null : <SummaryStat label={t("history.metricInterrupted")} value={pending.length} attention />}
        <SummaryStat label={t("history.metricRestorable")} value={restorableCount} />
      </SummaryStats>
      {problem === null ? null : (
        <Notice tone="danger">{saveProblemMessage(problem, t)}</Notice>
      )}
      {readFailed ? <Notice tone="danger">{t("history.readFailed")}</Notice> : null}
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
              <article key={item.id} className="grid grid-cols-[minmax(0,1fr)] gap-4 px-4 py-4 md:grid-cols-[minmax(0,1fr)_auto] md:items-center">
                <div className="min-w-0">
                  <p className="text-sm font-medium text-ink">{t(historyOperationLabelKey(item.operation, item.paths))}</p>
                  <p className="mt-1 text-xs text-ink-muted">
                    {t("history.interruptedDetail", {
                      operation: t(historyOperationLabelKey(item.operation, item.paths)),
                      startedAt: formatDateTime(item.startedAt, locale),
                      committed: item.committed,
                      total: item.paths.length,
                    })}
                  </p>
                  <p title={shownHistoryPaths(item.paths)} className="mt-2 truncate font-mono text-xs text-ink-muted empty:hidden">{shownHistoryPaths(item.paths)}</p>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Button kind="primary" onClick={() => setInspected(item)}>{t("history.viewChange")}</Button>
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
              <li key={entry.id} className={entryColumns}>
                <time dateTime={entry.startedAt} title={new Date(entry.startedAt).toLocaleString(locale)} className="text-xs text-ink-muted">{formatDateTime(entry.startedAt, locale)}</time>
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <p className="font-medium text-ink">{t(historyOperationLabelKey(entry.operation, entry.paths))}</p>
                    <Tag>{t(historyStatusLabelKey(entry.status))}</Tag>
                  </div>
                  <p title={shownHistoryPaths(entry.paths)} className="mt-1 truncate font-mono text-xs text-ink-faint empty:hidden">{shownHistoryPaths(entry.paths)}</p>
                </div>
                <div className="flex min-w-0 flex-wrap gap-2 md:justify-end">
                  {(entry.restorable ?? []).map((path) => (
                    <Button key={path} className={restoreButtonWraps} onClick={() => void restore(entry.id, path)}>
                      {t("history.restorePath", { path: restorableFileName(path, t) })}
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
      {inspected === null ? null : (
        <PendingChangeDialog
          transaction={inspected}
          onDismiss={() => setInspected(null)}
          onRecover={(action) => {
            setInspected(null);
            void recover(inspected.id, action);
          }}
        />
      )}
    </div>
  );
}
