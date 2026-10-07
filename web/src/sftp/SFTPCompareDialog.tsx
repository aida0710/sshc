import { useTranslate } from "../i18n/context";
import { comparisonStatusLabelKeys, entryTypeLabelKeys } from "./sftpMessageKeys";
import { formatBytes } from "../ui/format";
import { ModalShell } from "../ui/ModalShell";
import { Button, Notice } from "../ui/surface";
import { PanelState } from "../ui/PanelState";
import { localHostAlias } from "./localHost";

import type { SFTPLocation } from "./sftpLocation";
import { useSFTPComparison } from "./useSFTPComparison";

export function SFTPCompareDialog({ left, right, onDismiss }: { left: SFTPLocation; right: SFTPLocation; onDismiss: () => void }) {
  const t = useTranslate();
  const { comparison, changes, selected, problem, busy, readOnly, mode, setMode, toggle, copySelectedDifferences } = useSFTPComparison({ left, right, onCopied: onDismiss });
  const leftLabel = left.alias === localHostAlias ? t("sftp.local.connection") : left.alias;
  const rightLabel = right.alias === localHostAlias ? t("sftp.local.connection") : right.alias;

  return (
    <ModalShell labelledBy="sftp-compare-heading" onDismiss={onDismiss} panelClassName="flex h-[min(44rem,calc(100dvh-2rem))] w-full max-w-4xl flex-col overflow-hidden rounded-lg">
      <header className="border-b border-line/60 px-5 py-4">
        <h2 id="sftp-compare-heading" className="text-base font-semibold text-ink">{t("sftp.compare.heading")}</h2>
        <p className="mt-1 truncate font-mono text-xs text-ink-muted">{leftLabel}:{left.path} ⇄ {rightLabel}:{right.path}</p>
        {readOnly && mode === "content" ? null : <p className="mt-2 text-xs text-ink-muted">{t(readOnly ? "sftp.compare.readOnlyDescription" : "sftp.compare.description")}</p>}
        <label className="mt-3 flex items-center gap-2 text-sm text-ink-muted">
          {t("sftp.compare.mode")}
          <select aria-label={t("sftp.compare.mode")} value={mode} onChange={(event) => setMode(event.target.value === "content" ? "content" : "metadata")} className="rounded border border-control-line bg-control px-2 py-1">
            <option value="metadata">{t("sftp.compare.metadata")}</option>
            <option value="content">{t("sftp.compare.content")}</option>
          </select>
        </label>
        {mode === "content" ? <p className="mt-2 text-xs text-ink-muted">{t("sftp.compare.contentHint")}</p> : null}
        {comparison?.bytesRead === undefined ? null : <p className="mt-2 text-xs text-ink-muted">{t("sftp.search.bytesRead", { bytes: formatBytes(comparison.bytesRead) })}</p>}
        {comparison?.truncated ? <div className="mt-2"><Notice>{t("sftp.compare.partial")}</Notice></div> : null}
      </header>
      {problem === "" ? null : <div className="px-5 pt-3"><Notice tone="danger">{problem}</Notice></div>}
      <div className="min-h-0 flex-1 overflow-auto">
        {busy && comparison === null ? <PanelState tone="loading" title={t("sftp.compare.loading")} /> : null}
        {!busy && problem === "" && comparison !== null && changes.length === 0 ? <PanelState tone="empty" title={t(mode === "content" ? "sftp.compare.contentNoChanges" : "sftp.compare.noChanges")} /> : null}
        {changes.length === 0 ? null : <table className="w-full min-w-[38rem] text-left text-sm">
          <thead className="sticky top-0 bg-toolbar text-xs text-ink-muted"><tr>
            {readOnly ? null : <th className="w-10 px-3 py-2"><span className="sr-only">{t("sftp.selectAll")}</span></th>}
            <th className="px-2 py-2">{t("sftp.name")}</th>
            <th className="w-36 px-2 py-2">{t("sftp.compare.status")}</th>
            <th className="w-28 px-2 py-2 text-right">{leftLabel}</th>
            <th className="w-28 px-2 py-2 text-right">{rightLabel}</th>
          </tr></thead>
          <tbody>{changes.map((difference) => (
            <tr key={difference.relativePath} className="border-t border-line/40 hover:bg-select-fill/40">
              {readOnly ? null : <td className="px-3 py-2"><input type="checkbox" aria-label={t("sftp.selectEntry", { name: difference.relativePath })} disabled={busy || difference.status === "unverified"} checked={selected.has(difference.relativePath)} onChange={() => toggle(difference.relativePath)} className="size-4 accent-accent" /></td>}
              <td className="px-2 py-2 font-mono text-ink">{difference.relativePath}</td>
              <td className="px-2 py-2 text-xs text-ink-muted">{t(comparisonStatusLabelKeys[difference.status])}{difference.omission === undefined ? null : <span className="mt-1 block">{t(difference.omission === "byte_limit" ? "sftp.compare.skipBytes" : "sftp.compare.skipUnsupported")}</span>}</td>
              <td className="px-2 py-2 text-right text-xs text-ink-muted">{difference.left?.type === "file" ? formatBytes(difference.left.size) : difference.left === undefined ? "—" : t(entryTypeLabelKeys[difference.left.type])}</td>
              <td className="px-2 py-2 text-right text-xs text-ink-muted">{difference.right?.type === "file" ? formatBytes(difference.right.size) : difference.right === undefined ? "—" : t(entryTypeLabelKeys[difference.right.type])}</td>
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
  );
}
