import { useEffect, useMemo, useState } from "react";
import { useTranslate } from "../i18n/context";
import { sftpProblemText } from "./sftpProblemText";
import { comparisonStatusLabelKeys, entryTypeLabelKeys } from "./sftpMessageKeys";
import { formatBytes } from "../ui/format";
import { ModalShell } from "../ui/ModalShell";
import { Button, Notice } from "../ui/surface";
import { sftpApi, type DirectoryComparison } from "./api";
import { sftpTransferManager, type RemoteTransferSelection } from "./transferManager";
import { PanelState } from "../ui/PanelState";
import { localHostAlias } from "./localHost";

type Location = { alias: string; path: string };

function targetPath(root: string, relative: string): string {
  return `${root === "/" ? "" : root}/${relative}`;
}

export function SFTPCompareDialog({ left, right, onDismiss }: { left: Location; right: Location; onDismiss: () => void }) {
  const t = useTranslate();
  const readOnly = left.alias === localHostAlias || right.alias === localHostAlias;
  const leftLabel = left.alias === localHostAlias ? t("sftp.local.connection") : left.alias;
  const rightLabel = right.alias === localHostAlias ? t("sftp.local.connection") : right.alias;
  const [comparison, setComparison] = useState<DirectoryComparison | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [problem, setProblem] = useState("");
  const [busy, setBusy] = useState(true);

  useEffect(() => {
    let live = true;
    setBusy(true);
    sftpApi.compareDirectories(left.alias, left.path, right.alias, right.path).then((result) => {
      if (!live) return;
      setComparison(result);
      setSelected(new Set(result.entries.filter((entry) => entry.status !== "same").map((entry) => entry.relativePath)));
    }).catch((error) => {
      if (live) setProblem(sftpProblemText(t, error));
    }).finally(() => {
      if (live) setBusy(false);
    });
    return () => { live = false; };
  }, [left.alias, left.path, right.alias, right.path, t]);

  const changes = useMemo(() => comparison?.entries.filter((entry) => entry.status !== "same") ?? [], [comparison]);

  function toggle(relative: string) {
    setSelected((current) => {
      const next = new Set(current);
      if (next.has(relative)) next.delete(relative);
      else next.add(relative);
      return next;
    });
  }

  async function copySelectedDifferences(direction: "left" | "right") {
    if (comparison === null || readOnly) return;
    const sourceRoot = direction === "left" ? comparison.leftPath : comparison.rightPath;
    const destinationRoot = direction === "left" ? comparison.rightPath : comparison.leftPath;
    const sourceAlias = direction === "left" ? left.alias : right.alias;
    const targetAlias = direction === "left" ? right.alias : left.alias;
    const candidates = changes.filter((difference) => selected.has(difference.relativePath))
      .filter((difference) => (direction === "left" ? difference.left : difference.right) !== undefined);
    // Selecting a directory already copies all descendants. Suppress child jobs
    // below a selected directory so the preview cannot schedule duplicates.
    const selectedDirectories = candidates.flatMap((difference) => {
      const source = direction === "left" ? difference.left : difference.right;
      return source?.type === "directory" ? [difference.relativePath] : [];
    });
    const transfers: RemoteTransferSelection[] = candidates.flatMap((difference) => {
      const source = direction === "left" ? difference.left : difference.right;
      if (source === undefined || selectedDirectories.some((directory) => difference.relativePath !== directory && difference.relativePath.startsWith(`${directory}/`))) return [];
      return [{
        sourceAlias,
        sourcePath: `${sourceRoot === "/" ? "" : sourceRoot}/${difference.relativePath}`,
        targetAlias,
        targetPath: targetPath(destinationRoot, difference.relativePath),
        kind: source.type === "directory" ? "folder" : "file",
        name: source.name,
        totalBytes: source.type === "file" ? source.size : -1,
        overwrite: difference.status !== (direction === "left" ? "left_only" : "right_only"),
      }];
    });
    if (transfers.length === 0) return;
    setBusy(true);
    setProblem("");
    try {
      await sftpTransferManager.addRemoteTransfers(transfers, "copy");
      onDismiss();
    } catch (error) {
      setProblem(sftpProblemText(t, error));
      setBusy(false);
    }
  }

  return (
    <ModalShell labelledBy="sftp-compare-heading" onDismiss={onDismiss} panelClassName="flex h-[min(44rem,calc(100dvh-2rem))] w-full max-w-4xl flex-col overflow-hidden rounded-lg">
      <header className="border-b border-line/60 px-5 py-4">
        <h2 id="sftp-compare-heading" className="text-base font-semibold text-ink">{t("sftp.compare.heading")}</h2>
        <p className="mt-1 truncate font-mono text-xs text-ink-muted">{leftLabel}:{left.path} ⇄ {rightLabel}:{right.path}</p>
        <p className="mt-2 text-xs text-ink-muted">{t(readOnly ? "sftp.compare.readOnlyDescription" : "sftp.compare.description")}</p>
      </header>
      {problem === "" ? null : <div className="px-5 pt-3"><Notice tone="danger">{problem}</Notice></div>}
      <div className="min-h-0 flex-1 overflow-auto">
        {busy && comparison === null ? <PanelState tone="loading" title={t("sftp.compare.loading")} /> : null}
        {!busy && problem === "" && comparison !== null && changes.length === 0 ? <PanelState tone="empty" title={t("sftp.compare.noChanges")} /> : null}
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
              {readOnly ? null : <td className="px-3 py-2"><input type="checkbox" aria-label={t("sftp.selectEntry", { name: difference.relativePath })} checked={selected.has(difference.relativePath)} onChange={() => toggle(difference.relativePath)} className="size-4 accent-accent" /></td>}
              <td className="px-2 py-2 font-mono text-ink">{difference.relativePath}</td>
              <td className="px-2 py-2 text-xs text-ink-muted">{t(comparisonStatusLabelKeys[difference.status])}</td>
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
