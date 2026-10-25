import { useEffect, useId, useState, type ReactNode, type RefObject } from "react";
import { DisclosureSummary } from "../ui/DisclosureSummary";
import { useTranslate } from "../i18n/context";
import { mobileViewportQuery, useMediaQuery } from "../ui/useMediaQuery";
import { ModalShell } from "../ui/ModalShell";
import { Button } from "../ui/surface";
import { localHostAlias } from "./localHost";
import { sftpApi, type RemoteDirectoryStats, type RemoteEntry } from "./api";
import { formatBytes } from "../ui/format";
import { entryKind } from "./entryKind";
import { entryTypeLabelKeys } from "./sftpMessageKeys";
import { symbolicModeToOctal } from "./transfers";
import { FilePreviewBody } from "./FilePreviewBody";
import { useFilePreview } from "./useFilePreview";

type DirectoryStatsState =
  | { kind: "idle" }
  | { kind: "loading" }
  | { kind: "loaded"; stats: RemoteDirectoryStats }
  | { kind: "unavailable" };

function Property({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="border-t border-hairline py-1.5 first:border-t-0">
      <dt className="text-[11px] uppercase tracking-wide text-ink-faint">{label}</dt>
      <dd className="break-all font-mono text-xs text-ink">{children}</dd>
    </div>
  );
}

export function SFTPDetailsDialog({
  alias,
  entries,
  busy,
  onClose,
  onEdit,
  onDownload,
  canDownload,
  onRename,
  onChangeLinkTarget,
  onChangeOwnership,
  returnFocusRef,
}: {
  alias: string;
  entries: RemoteEntry[];
  busy: boolean;
  onClose: () => void;
  onEdit: (entry: RemoteEntry) => void;
  onDownload: (entries: RemoteEntry[]) => void;
  // Whether "send it over" can take an entry; the pane knows where it goes.
  canDownload: (entry: RemoteEntry) => boolean;
  onRename: (entry: RemoteEntry) => void;
  onChangeLinkTarget?: ((entry: RemoteEntry) => void) | undefined;
  onChangeOwnership?: ((entry: RemoteEntry) => void) | undefined;
  returnFocusRef?: RefObject<HTMLElement | null>;
}) {
  const t = useTranslate();
  const headingId = useId();
  const mobileViewport = useMediaQuery(mobileViewportQuery);
  const [directoryStats, setDirectoryStats] = useState<DirectoryStatsState>({ kind: "idle" });
  const entry = entries.length === 1 ? entries[0] ?? null : null;
  const kind = entry === null ? null : entryKind(entry);
  const previewPath = entry !== null && kind === "file" ? entry.path : null;
  const directoryPath = entry !== null && kind === "directory" && alias !== localHostAlias ? entry.path : null;
  const totalBytes = entries.reduce((sum, item) => sum + (entryKind(item) === "file" ? item.size : 0), 0);
  const preview = useFilePreview(alias, previewPath);
  const heading = entry === null ? t("sftp.detailsForCount", { count: entries.length }) : t("sftp.detailsFor", { name: entry.name });

  useEffect(() => {
    if (directoryPath === null) {
      setDirectoryStats({ kind: "idle" });
      return;
    }
    let active = true;
    setDirectoryStats({ kind: "loading" });
    void sftpApi.directoryStats(alias, directoryPath).then(
      (stats) => { if (active) setDirectoryStats({ kind: "loaded", stats }); },
      () => { if (active) setDirectoryStats({ kind: "unavailable" }); },
    );
    return () => { active = false; };
  }, [alias, directoryPath]);

  const properties = (
        <dl role="group" className="min-h-0 overflow-auto border-t border-line px-3 py-2 md:border-l md:border-t-0" aria-label={t("sftp.properties")}>
          {entry === null ? (
            <>
              <Property label={t("sftp.selectedItems")}>{entries.length.toLocaleString()}</Property>
              <Property label={t("sftp.totalSize")}>{`${totalBytes.toLocaleString()} (${formatBytes(totalBytes)})`}</Property>
            </>
          ) : (
            <>
              <Property label={t("sftp.path")}>{entry.path}</Property>
              <Property label={t("sftp.type")}>{t(entryTypeLabelKeys[entry.type])}</Property>
              {entry.type === "symlink" ? (
                <Property label={t("sftp.linkTarget")}>
                  {entry.targetType === undefined ? t("sftp.brokenLink", { target: entry.linkTarget ?? "" }) : entry.linkTarget ?? ""}
                </Property>
              ) : null}
              <Property label={t("sftp.size")}>
                {kind === "file"
                  ? `${entry.size.toLocaleString()} (${formatBytes(entry.size)})`
                  : directoryStats.kind === "loading"
                    ? t("sftp.calculating")
                    : directoryStats.kind === "loaded"
                      ? `${directoryStats.stats.truncated ? t("sftp.partialSizePrefix") : ""}${directoryStats.stats.bytes.toLocaleString()} (${formatBytes(directoryStats.stats.bytes)})`
                      : t("sftp.sizeUnavailable")}
              </Property>
              {kind === "directory" && directoryStats.kind === "loaded" ? (
                <>
                  <Property label={t("sftp.containedFiles")}>{directoryStats.stats.files.toLocaleString()}</Property>
                  <Property label={t("sftp.containedDirectories")}>{Math.max(0, directoryStats.stats.directories - 1).toLocaleString()}</Property>
                </>
              ) : null}
              <Property label={t("sftp.modified")}>
                <time dateTime={entry.modifiedAt}>{new Date(entry.modifiedAt).toLocaleString()}</time>
              </Property>
              <Property label={t("sftp.permissions")}>{`${entry.mode} (${symbolicModeToOctal(entry.mode)})`}</Property>
              <Property label={t("sftp.uid")}>{entry.uid ?? t("sftp.ownershipUnavailable")}</Property>
              <Property label={t("sftp.gid")}>{entry.gid ?? t("sftp.ownershipUnavailable")}</Property>
              <Property label={t("sftp.revision")}>{entry.revision}</Property>
            </>
          )}
        </dl>
  );

  return (
    <ModalShell
      labelledBy={headingId}
      onDismiss={onClose}
      {...(returnFocusRef === undefined ? {} : { returnFocusRef })}
      panelClassName="flex h-[min(46rem,calc(100dvh-2rem))] w-full max-w-4xl flex-col overflow-hidden rounded-lg"
    >
      <div className="flex items-center gap-2 border-b border-line bg-toolbar px-3 py-2">
        <h2 id={headingId} className="min-w-0 grow truncate text-sm font-medium">{heading}</h2>
        <button type="button" className="text-xs text-ink-muted hover:text-ink" onClick={onClose}>{t("sftp.close")}</button>
      </div>

      <div className={mobileViewport ? "flex min-h-0 flex-1 flex-col overflow-hidden" : "grid min-h-0 flex-1 grid-cols-[minmax(0,1fr)_18rem] overflow-hidden"}>
        <div role="group" className="min-h-0 min-w-0 flex-1 overflow-auto overscroll-contain bg-surface-subtle p-3" aria-label={t("sftp.preview")}>
          {entry === null ? (
            <ul className="space-y-1 text-xs" aria-label={t("sftp.selectedItems")}>
              {entries.map((item) => (
                <li key={item.path} className="break-all font-mono text-ink-muted">{item.path}</li>
              ))}
            </ul>
          ) : preview.kind !== "idle" ? (
            <FilePreviewBody preview={preview} path={entry.path} name={entry.name} />
          ) : (
            <p className="text-sm text-ink-muted">{t(entryTypeLabelKeys[entry.type])}</p>
          )}
        </div>

        {mobileViewport ? <details className="min-h-0 shrink-0 overflow-auto border-t border-line"><DisclosureSummary className="px-3 py-3 text-sm text-ink-muted">{t("sftp.properties")}</DisclosureSummary><div className="max-h-40 overflow-auto">{properties}</div></details> : properties}
      </div>

      <div className="flex flex-wrap justify-end gap-2 border-t border-line px-3 py-2">
        {entry !== null && kind === "file"
          ? <Button disabled={busy} onClick={() => onEdit(entry)}>{t("sftp.editFile")}</Button>
          : null}
        {entries.some(canDownload)
          ? <Button disabled={busy} onClick={() => onDownload(entries)}>{t("sftp.download")}</Button>
          : null}
        {entry?.type === "symlink" && onChangeLinkTarget ? <Button disabled={busy} onClick={() => onChangeLinkTarget(entry)}>{t("sftp.changeLinkTarget")}</Button> : null}
        {entry !== null && (entry.type === "file" || entry.type === "directory") && onChangeOwnership ? <Button disabled={busy || entry.uid === undefined || entry.gid === undefined} onClick={() => onChangeOwnership(entry)}>{t("sftp.changeOwnership")}</Button> : null}
        {entry === null ? null : <Button disabled={busy} onClick={() => onRename(entry)}>{t("sftp.rename")}</Button>}
      </div>
    </ModalShell>
  );
}
