import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { sftpTransferManager, type ManagedTransferJob } from "./transferManager";
import { TransferJobRow } from "./TransferJobRow";

export function TransferBatchList({ batches, waiting, processingStopped, maxReconnectAttempts, runControl }: {
  batches: readonly [string, ManagedTransferJob[]][];
  waiting: readonly ManagedTransferJob[];
  processingStopped: boolean;
  maxReconnectAttempts: number;
  runControl: (operation: () => Promise<void>) => void;
}) {
  const t = useTranslate();
  return batches.map(([batchId, items]) => {
    const first = items[0]!;
    const failed = items.filter((item) => item.status === "failed").length;
    const completed = items.filter((item) => item.status === "completed").length;
    return <section key={batchId} className="min-w-0 overflow-hidden rounded-md bg-surface-subtle/70" aria-label={first.batchName}>
      {items.length > 1 ? <div className="grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-2 border-b border-line/50 px-3 py-2 text-xs md:grid-cols-[minmax(10rem,1.3fr)_minmax(8rem,1fr)_auto]">
        <div className="flex min-w-0 items-center gap-2">
          <Icon name={first.batchKind === "folder" ? "folder" : "files"} className="size-3.5 text-ink-muted" />
          <span className="truncate font-medium" title={first.batchName}>{first.batchName}</span>
        </div>
        <span className="shrink-0 tabular-nums text-ink-muted">{t("sftp.manager.batchProgress", { completed, total: items.length })}</span>
        {failed > 0 ? <button type="button" className="min-h-11 shrink-0 px-2 text-accent md:min-h-8 [@media(pointer:coarse)]:min-h-11" onClick={() => runControl(() => sftpTransferManager.retryFailed(batchId))}>{t("sftp.manager.retryFailed", { count: failed })}</button> : null}
      </div> : null}
      <ul aria-label={t("sftp.manager.items")}>
        {items.map((job) => <TransferJobRow key={job.id} job={job} waiting={waiting} processingStopped={processingStopped}
          maxReconnectAttempts={maxReconnectAttempts} runControl={runControl} />)}
      </ul>
    </section>;
  });
}
