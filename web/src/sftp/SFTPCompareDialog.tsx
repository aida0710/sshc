import { useEffect, useMemo, useState } from "react";
import { failureCode } from "../api/client";
import { useTranslate } from "../i18n/context";
import { ModalShell } from "../ui/ModalShell";
import { Button } from "../ui/surface";
import { sftpApi, type DirectoryComparison } from "./api";
import { sftpTransferManager, type RemoteTransferSelection } from "./transferManager";
import { PanelState } from "../ui/PanelState";

type Location = { alias: string; path: string };

function targetPath(root: string, relative: string): string {
  return `${root === "/" ? "" : root}/${relative}`;
}

export function SFTPCompareDialog({ left, right, onDismiss }: { left: Location; right: Location; onDismiss: () => void }) {
  const t = useTranslate();
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
      if (live) setProblem(failureCode(error) || "sftp_failed");
    }).finally(() => {
      if (live) setBusy(false);
    });
    return () => { live = false; };
  }, [left.alias, left.path, right.alias, right.path]);

  const changes = useMemo(() => comparison?.entries.filter((entry) => entry.status !== "same") ?? [], [comparison]);

  function toggle(relative: string) {
    setSelected((current) => {
      const next = new Set(current);
      if (next.has(relative)) next.delete(relative);
      else next.add(relative);
      return next;
    });
  }

  async function synchronize(direction: "left" | "right") {
    if (comparison === null) return;
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
      setProblem(failureCode(error) || (error instanceof Error ? error.message : "sftp_failed"));
      setBusy(false);
    }
  }

  return (
    <ModalShell labelledBy="sftp-compare-heading" onDismiss={onDismiss} panelClassName="flex h-[min(44rem,calc(100dvh-2rem))] w-full max-w-4xl flex-col overflow-hidden rounded-lg">
      <header className="border-b border-line/60 px-5 py-4">
        <h2 id="sftp-compare-heading" className="text-base font-semibold text-ink">{t("sftp.compare.heading")}</h2>
        <p className="mt-1 truncate font-mono text-xs text-ink-muted">{left.alias}:{left.path} ⇄ {right.alias}:{right.path}</p>
        <p className="mt-2 text-xs text-ink-muted">{t("sftp.compare.description")}</p>
      </header>
      {problem === "" ? null : <p role="alert" className="border-b border-notice-line bg-notice px-5 py-2 text-sm text-notice-ink">{problem}</p>}
      <div className="min-h-0 flex-1 overflow-auto">
        {busy && comparison === null ? <PanelState tone="loading" title={t("sftp.compare.loading")} /> : null}
        {!busy && changes.length === 0 ? <PanelState tone="empty" title={t("sftp.compare.noChanges")} /> : null}
        {changes.length === 0 ? null : <table className="w-full min-w-[38rem] text-left text-sm">
          <thead className="sticky top-0 bg-toolbar text-xs text-ink-muted"><tr>
            <th className="w-10 px-3 py-2"><span className="sr-only">{t("sftp.selectAll")}</span></th>
            <th className="px-2 py-2">{t("sftp.name")}</th>
            <th className="w-36 px-2 py-2">{t("sftp.compare.status")}</th>
            <th className="w-28 px-2 py-2 text-right">{left.alias}</th>
            <th className="w-28 px-2 py-2 text-right">{right.alias}</th>
          </tr></thead>
          <tbody>{changes.map((difference) => (
            <tr key={difference.relativePath} className="border-t border-line/40 hover:bg-select-fill/40">
              <td className="px-3 py-2"><input type="checkbox" checked={selected.has(difference.relativePath)} onChange={() => toggle(difference.relativePath)} className="size-4 accent-accent" /></td>
              <td className="px-2 py-2 font-mono text-ink">{difference.relativePath}</td>
              <td className="px-2 py-2 text-xs text-ink-muted">{t(`sftp.compare.${difference.status}`)}</td>
              <td className="px-2 py-2 text-right text-xs text-ink-muted">{difference.left?.type === "file" ? difference.left.size.toLocaleString() : difference.left === undefined ? "—" : t(`sftp.type.${difference.left.type}`)}</td>
              <td className="px-2 py-2 text-right text-xs text-ink-muted">{difference.right?.type === "file" ? difference.right.size.toLocaleString() : difference.right === undefined ? "—" : t(`sftp.type.${difference.right.type}`)}</td>
            </tr>
          ))}</tbody>
        </table>}
      </div>
      <footer className="flex flex-wrap justify-end gap-2 border-t border-line/60 px-5 py-4">
        <Button onClick={onDismiss}>{t("sftp.cancel")}</Button>
        <Button disabled={busy || selected.size === 0} onClick={() => void synchronize("right")}>{t("sftp.compare.rightToLeft")}</Button>
        <Button kind="primary" disabled={busy || selected.size === 0} onClick={() => void synchronize("left")}>{t("sftp.compare.leftToRight")}</Button>
      </footer>
    </ModalShell>
  );
}
