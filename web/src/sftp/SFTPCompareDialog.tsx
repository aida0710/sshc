import { dialogHeading } from "../ui/form";
import { useState } from "react";
import { useTranslate } from "../i18n/context";
import { DisclosureSummary } from "../ui/DisclosureSummary";
import { SFTPTextCompareDialog } from "./SFTPTextCompareDialog";
import type { DirectoryComparison } from "./api";
import { comparisonStatusLabelKeys, entryTypeLabelKeys } from "./sftpMessageKeys";
import { formatBytes } from "../ui/format";
import { ModalShell } from "../ui/ModalShell";
import { Button, Notice } from "../ui/surface";
import { PanelState } from "../ui/PanelState";
import { PathText } from "../ui/PathText";
import { mobileViewportQuery, useMediaQuery } from "../ui/useMediaQuery";
import { localHostAlias } from "./localHost";

import type { SFTPLocation } from "./sftpLocation";
import { useSFTPComparison } from "./useSFTPComparison";

type DirectoryDifference = DirectoryComparison["entries"][number];

// What one side has at a path: a file's size, or the kind of what is there.
function sideFact(entry: DirectoryDifference["left"], t: ReturnType<typeof useTranslate>): string {
  if (entry === undefined) return "—";
  return entry.type === "file" ? formatBytes(entry.size) : t(entryTypeLabelKeys[entry.type]);
}

function canCompareText(difference: DirectoryDifference): boolean {
  if (difference.status === "unverified") return false;
  const sides = [difference.left, difference.right];
  return sides.some((entry) => entry?.type === "file") && sides.every((entry) => entry === undefined || entry.type === "file");
}

export function SFTPCompareDialog({ left, right, onDismiss }: { left: SFTPLocation; right: SFTPLocation; onDismiss: () => void }) {
  const t = useTranslate();
  // A phone has no width for a column per side: each side's fact goes under the name, and the table fits the screen.
  const compact = useMediaQuery(mobileViewportQuery);
  const [textDifference, setTextDifference] = useState<DirectoryDifference | null>(null);
  const { comparison, changes, selected, problem, busy, readOnly, mode, setMode, toggle, copySelectedDifferences } = useSFTPComparison({ left, right, onCopied: onDismiss });
  const leftLabel = left.alias === localHostAlias ? t("sftp.local.connection") : left.alias;
  const rightLabel = right.alias === localHostAlias ? t("sftp.local.connection") : right.alias;

  return (
    <>
      <ModalShell labelledBy="sftp-compare-heading" onDismiss={onDismiss} panelClassName="flex h-[min(44rem,calc(100dvh-2rem))] w-full max-w-4xl flex-col overflow-hidden">
        <header className="border-b border-line/60 px-5 py-4">
          <h2 id="sftp-compare-heading" className={dialogHeading}>{t("sftp.compare.heading")}</h2>
          <p className="mt-1 flex min-w-0 flex-wrap items-center gap-x-2 font-mono text-xs text-ink-muted">
            <PathText path={`${leftLabel}:${left.path}`} className="max-w-full" />
            <span aria-hidden="true">⇄</span>
            <PathText path={`${rightLabel}:${right.path}`} className="max-w-full" />
          </p>
          <details className="mt-2 text-xs text-ink-muted"><DisclosureSummary>{t("sftp.compare.explanation")}</DisclosureSummary><p className="mt-2">{t(readOnly ? "sftp.compare.readOnlyDescription" : "sftp.compare.description")}</p>{mode === "content" ? <p className="mt-2">{t("sftp.compare.contentHint")}</p> : null}</details>
          <label className="mt-3 flex items-center gap-2 text-sm text-ink-muted">
            {t("sftp.compare.mode")}
            <select aria-label={t("sftp.compare.mode")} value={mode} onChange={(event) => setMode(event.target.value === "content" ? "content" : "metadata")} className="rounded-md border border-control-line bg-control px-2 py-1">
              <option value="metadata">{t("sftp.compare.metadata")}</option>
              <option value="content">{t("sftp.compare.content")}</option>
            </select>
          </label>
          {comparison?.bytesRead === undefined ? null : <p className="mt-2 text-xs text-ink-muted">{t("sftp.search.bytesRead", { bytes: formatBytes(comparison.bytesRead) })}</p>}
          {comparison?.truncated ? <div className="mt-2"><Notice>{t("sftp.compare.partial")}</Notice></div> : null}
        </header>
        {problem === "" ? null : <div className="px-5 pt-3"><Notice tone="danger">{problem}</Notice></div>}
        <div className="min-h-0 flex-1 overflow-auto">
          {busy && comparison === null ? <PanelState tone="loading" title={t("sftp.compare.loading")} /> : null}
          {!busy && problem === "" && comparison !== null && changes.length === 0 ? <PanelState tone="empty" title={t(mode === "content" ? "sftp.compare.contentNoChanges" : "sftp.compare.noChanges")} /> : null}
          {changes.length === 0 ? null : <table className={`w-full text-left text-sm ${compact ? "table-fixed" : "min-w-[38rem]"}`}>
            <thead className="sticky top-0 bg-toolbar text-xs text-ink-muted"><tr>
              {readOnly ? null : <th className="w-10 px-3 py-2"><span className="sr-only">{t("sftp.selectAll")}</span></th>}
              <th className="px-2 py-2">{t("sftp.name")}</th>
              <th className={`px-2 py-2 ${compact ? "w-20" : "w-36"}`}>{t("sftp.compare.status")}</th>
              {compact ? null : <>
                <th className="w-28 px-2 py-2 text-right">{leftLabel}</th>
                <th className="w-28 px-2 py-2 text-right">{rightLabel}</th>
              </>}
            </tr></thead>
            <tbody>{changes.map((difference) => (
              <tr key={difference.relativePath} className="border-t border-line/40 hover:bg-select-fill/40">
                {readOnly ? null : <td className="px-3 py-2"><input type="checkbox" aria-label={t("sftp.selectEntry", { name: difference.relativePath })} disabled={busy || difference.status === "unverified"} checked={selected.has(difference.relativePath)} onChange={() => toggle(difference.relativePath)} className="size-4 accent-accent" /></td>}
                <td className="px-2 py-2 font-mono text-ink [overflow-wrap:anywhere]">{difference.relativePath}
                  {compact ? <span className="mt-1 block font-sans text-xs text-ink-muted">{leftLabel}: {sideFact(difference.left, t)} · {rightLabel}: {sideFact(difference.right, t)}</span> : null}
                  {canCompareText(difference) ? <button type="button" className="mt-1 block text-xs text-accent hover:underline" aria-label={t("sftp.compare.viewTextFor", { path: difference.relativePath })} onClick={() => setTextDifference(difference)}>{t("sftp.compare.viewText")}</button> : null}
                </td>
                <td className="px-2 py-2 text-xs text-ink-muted">{t(comparisonStatusLabelKeys[difference.status])}{difference.omission === undefined ? null : <span className="mt-1 block">{t(difference.omission === "byte_limit" ? "sftp.compare.skipBytes" : "sftp.compare.skipUnsupported")}</span>}</td>
                {compact ? null : <>
                  <td className="px-2 py-2 text-right text-xs text-ink-muted">{sideFact(difference.left, t)}</td>
                  <td className="px-2 py-2 text-right text-xs text-ink-muted">{sideFact(difference.right, t)}</td>
                </>}
              </tr>
            ))}</tbody>
          </table>}
        </div>
        <footer className="flex flex-wrap justify-end gap-2 border-t border-line/60 px-5 py-4">
          <Button onClick={onDismiss}>{t("sftp.cancel")}</Button>
          {readOnly ? null : <>
            <Button disabled={busy || selected.size === 0} onClick={() => void copySelectedDifferences("right")}>{t("sftp.compare.rightToLeft")}</Button>
            <Button kind="primary" disabled={busy || selected.size === 0} onClick={() => void copySelectedDifferences("left")}>{t("sftp.compare.leftToRight")}</Button>
          </>}
        </footer>
      </ModalShell>
      {textDifference === null ? null : <SFTPTextCompareDialog path={textDifference.relativePath} left={{ alias: left.alias, entry: textDifference.left }} right={{ alias: right.alias, entry: textDifference.right }} onDismiss={() => setTextDifference(null)} />}
    </>
  );
}
