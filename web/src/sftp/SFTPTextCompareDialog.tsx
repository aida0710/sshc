import { lazy, Suspense, useId } from "react";
import { useTranslate } from "../i18n/context";
import { ModalShell } from "../ui/ModalShell";
import { PanelState } from "../ui/PanelState";
import { Button, Notice } from "../ui/surface";
import { localHostAlias } from "./localHost";
import { loadMonacoDiffEditor } from "./loadMonacoEditor";
import { useSFTPTextComparison, type TextComparisonSide } from "./useSFTPTextComparison";

const MonacoDiffEditor = lazy(() => loadMonacoDiffEditor().then(({ MonacoDiffEditor }) => ({ default: MonacoDiffEditor })));

export function SFTPTextCompareDialog({ path, left, right, onDismiss }: { path: string; left: TextComparisonSide; right: TextComparisonSide; onDismiss: () => void }) {
  const t = useTranslate();
  const id = useId();
  const { comparison, problem } = useSFTPTextComparison({ left, right });
  const label = (side: TextComparisonSide) => `${side.alias === localHostAlias ? t("sftp.local.connection") : side.alias}:${side.entry?.path ?? t("sftp.compare.absent")}`;
  return <ModalShell labelledBy={id} onDismiss={onDismiss} panelClassName="flex h-[min(52rem,calc(100dvh-2rem))] w-full max-w-6xl flex-col overflow-hidden rounded-lg">
    <header className="border-b border-line bg-toolbar px-3 py-2">
      <h2 id={id} className="truncate text-sm font-medium">{t("sftp.compare.textHeading", { path })}</h2>
      <div className="mt-1 flex flex-wrap gap-x-3 break-all font-mono text-xs text-ink-muted"><span>{label(left)}</span><span>→</span><span>{label(right)}</span></div>
    </header>
    <div className="min-h-0 flex-1">
      {problem !== "" ? <div className="p-3"><Notice tone="danger">{problem}</Notice></div> : comparison === null ? <PanelState tone="loading" title={t("sftp.compare.loading")} /> : <Suspense fallback={<PanelState tone="loading" title={t("sftp.compare.loading")} />}><MonacoDiffEditor path={path} original={comparison.original} modified={comparison.modified} /></Suspense>}
    </div>
    <footer className="flex justify-end border-t border-line px-3 py-2"><Button onClick={onDismiss}>{t("sftp.close")}</Button></footer>
  </ModalShell>;
}
