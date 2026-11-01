import { lazy, Suspense, useEffect, useId, useState } from "react";
import { configApi, type PendingChanges, type PendingFileChange, type PendingTransaction } from "../api/config";
import type { Problem } from "../api/client";
import { toProblem } from "../api/guards";
import { loadMonacoDiffEditor } from "../editor/loadMonacoEditor";
import { useLanguage } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { formatDateTime } from "../ui/format";
import { ModalShell } from "../ui/ModalShell";
import { PanelState } from "../ui/PanelState";
import { Button, Notice } from "../ui/surface";
import { Tag } from "../ui/Tag";
import { historyOperationLabelKey } from "./historyLabels";

const MonacoDiffEditor = lazy(() => loadMonacoDiffEditor().then(({ MonacoDiffEditor }) => ({ default: MonacoDiffEditor })));

type HiddenContents = Exclude<PendingFileChange["contents"], "text" | "none">;

// hiddenContentsMessages says why a file is not compared, keyed by the reason the engine gives.
const hiddenContentsMessages: Record<HiddenContents, MessageKey> = {
  secret: "history.change.secret",
  binary: "history.change.binary",
  too_large: "history.change.tooLarge",
  vault_locked: "history.change.vaultLocked",
  unavailable: "history.change.unavailable",
};

// actionMessages describe the steps that carry no contents to compare.
const actionMessages: Record<Exclude<PendingFileChange["action"], "write" | "remove">, MessageKey> = {
  move: "history.change.move",
  mkdir: "history.change.makeDirectory",
  rmdir: "history.change.removeDirectory",
  note: "history.change.note",
};

export function PendingChangeDialog({
  transaction,
  onRecover,
  onDismiss,
}: {
  transaction: PendingTransaction;
  onRecover: (action: "complete" | "rollback") => void;
  onDismiss: () => void;
}) {
  const { t, locale } = useLanguage();
  const headingId = useId();
  const [changes, setChanges] = useState<PendingChanges | null>(null);
  const [problem, setProblem] = useState<Problem | null>(null);
  const [selected, setSelected] = useState(0);

  useEffect(() => {
    let current = true;
    configApi.pendingChanges(transaction.id).then(
      (value) => { if (current) setChanges(value); },
      (error: unknown) => { if (current) setProblem(toProblem(error)); },
    );
    return () => { current = false; };
  }, [transaction.id]);

  const files = changes?.files ?? [];
  const file = files[selected];

  function fileNote(item: PendingFileChange): string {
    if (item.contents !== "text" && item.contents !== "none") return t("history.change.hiddenShort");
    if (!item.beforeExists && item.afterExists) return t("history.change.newFile");
    if (item.beforeExists && !item.afterExists) return t("history.change.removed");
    return "";
  }

  function fileBody(item: PendingFileChange) {
    if (item.contents === "text") {
      return (
        <div className="flex h-full min-h-0 flex-col">
          <p className="border-b border-line px-3 py-2 font-mono text-xs text-ink-muted">
            {t("history.change.sides", { path: item.path })}
            {fileNote(item) === "" ? null : <span className="ml-2 font-sans">{fileNote(item)}</span>}
          </p>
          <div className="min-h-0 flex-1">
            <Suspense fallback={<PanelState tone="loading" title={t("history.change.loading")} />}>
              <MonacoDiffEditor path={item.path} original={item.before ?? ""} modified={item.after ?? ""} />
            </Suspense>
          </div>
        </div>
      );
    }
    if (item.contents === "none") {
      const key = item.action === "write" || item.action === "remove" ? "history.change.unavailable" : actionMessages[item.action];
      return <PanelState tone="empty" title={t(key, { target: item.target ?? "" })} />;
    }
    return <PanelState tone="empty" title={t(hiddenContentsMessages[item.contents])} />;
  }

  function body() {
    if (problem !== null) {
      return <div className="p-3"><Notice tone="danger">{t("history.requestRejected", { code: problem.code })}</Notice></div>;
    }
    if (changes === null) return <PanelState tone="loading" title={t("history.change.loading")} />;
    if (file === undefined) return <PanelState tone="empty" title={t("history.change.empty")} />;
    return fileBody(file);
  }

  return (
    <ModalShell
      labelledBy={headingId}
      onDismiss={onDismiss}
      panelClassName="flex h-[min(44rem,calc(100dvh-2rem))] w-full max-w-6xl flex-col overflow-hidden rounded-lg"
    >
      <header className="border-b border-line bg-toolbar px-4 py-3">
        <h2 id={headingId} className="truncate text-sm font-medium">
          {t("history.change.heading", { operation: t(historyOperationLabelKey(transaction.operation)) })}
        </h2>
        <p className="mt-1 text-xs text-ink-muted">
          {t("history.change.summary", {
            startedAt: formatDateTime(transaction.startedAt, locale),
            committed: transaction.committed,
            total: transaction.paths.length,
          })}
        </p>
      </header>
      <div className="grid min-h-0 flex-1 grid-rows-[auto_minmax(0,1fr)] md:grid-cols-[17rem_minmax(0,1fr)] md:grid-rows-1">
        <nav aria-label={t("history.change.files")} className="max-h-40 overflow-auto border-b border-line p-2 md:max-h-none md:border-b-0 md:border-r">
          {files.map((item, index) => (
            <button
              key={item.path}
              type="button"
              aria-current={index === selected ? "true" : undefined}
              onClick={() => setSelected(index)}
              className={`flex w-full flex-col items-start gap-1 rounded-md px-3 py-2 text-left ${index === selected ? "bg-select-fill" : "hover:bg-hover"}`}
            >
              <span className="w-full truncate font-mono text-xs text-ink">{item.path}</span>
              <span className="flex flex-wrap items-center gap-2 text-xs text-ink-muted">
                <Tag tone={item.committed ? "neutral" : "warn"}>{t(item.committed ? "history.change.written" : "history.change.notYet")}</Tag>
                {fileNote(item)}
              </span>
            </button>
          ))}
        </nav>
        <div className="min-h-0">{body()}</div>
      </div>
      <footer className="flex flex-wrap items-center gap-2 border-t border-line px-4 py-3">
        <p className="w-full text-xs text-ink-muted md:w-auto md:min-w-0 md:flex-1">{t("history.change.choice")}</p>
        <Button onClick={onDismiss}>{t("history.close")}</Button>
        <Button onClick={() => onRecover("rollback")}>{t("history.rollBack")}</Button>
        <Button disabled={!transaction.canComplete} onClick={() => onRecover("complete")}>{t("history.complete")}</Button>
      </footer>
    </ModalShell>
  );
}
