import { useCallback, useId, type ReactNode, type RefObject } from "react";
import { DisclosureSummary } from "../ui/DisclosureSummary";
import { useLanguage } from "../i18n/context";
import { mobileViewportQuery, useMediaQuery } from "../ui/useMediaQuery";
import { ActionMenu } from "../ui/ActionMenu";
import { ModalShell } from "../ui/ModalShell";
import { Button } from "../ui/surface";
import { localHostAlias } from "./localHost";
import { sftpApi, type RemoteDirectoryStats, type RemoteEntry } from "./api";
import { formatBytes, formatDateTime } from "../ui/format";
import { entryKind } from "./entryKind";
import { entryTypeLabelKeys } from "./sftpMessageKeys";
import { symbolicModeToOctal } from "./transfers";
import { FilePreviewBody } from "./FilePreviewBody";
import { PreviewIconButton } from "./PreviewIconButton";
import { PreviewStepper } from "./PreviewStepper";
import { useFilePreview } from "./useFilePreview";
import { usePreviewStepping } from "./usePreviewStepping";
import { useLoad } from "../ui/useLoad";

function Property({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="border-t border-hairline py-1.5 first:border-t-0">
      <dt className="text-[11px] uppercase tracking-wide text-ink-faint">{label}</dt>
      <dd className="break-all font-mono text-xs text-ink">{children}</dd>
    </div>
  );
}

// SFTPDetailsDialog shows the properties of the inspected entries, and a preview of a single file. steps are
// the files of the pane in the order it shows them: the step buttons, the arrow keys and a finger's swipe
// across the previewed picture go through them, asking the pane to inspect another (onStep).
export function SFTPDetailsDialog({
  alias,
  entries,
  steps = [],
  onStep,
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
  steps?: RemoteEntry[];
  onStep?: (entry: RemoteEntry) => void;
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
  const { locale, t } = useLanguage();
  const headingId = useId();
  const mobileViewport = useMediaQuery(mobileViewportQuery);
  const entry = entries.length === 1 ? entries[0] ?? null : null;
  const kind = entry === null ? null : entryKind(entry);
  const previewPath = entry !== null && kind === "file" ? entry.path : null;
  const directoryPath = entry !== null && kind === "directory" && alias !== localHostAlias ? entry.path : null;
  const totalBytes = entries.reduce((sum, item) => sum + (entryKind(item) === "file" ? item.size : 0), 0);
  const preview = useFilePreview(alias, previewPath);
  // An entry that is not among the steps (a folder, or several entries) is shown alone.
  const stepIndex = entry === null || onStep === undefined ? -1 : steps.findIndex((item) => item.path === entry.path);
  const stepPosition = Math.max(0, stepIndex);
  const stepCount = stepIndex < 0 ? 1 : steps.length;
  const stepTo = (position: number) => {
    const next = steps[position];
    if (next !== undefined) onStep?.(next);
  };
  const onSwipe = usePreviewStepping({ position: stepPosition, count: stepCount, onStep: stepTo, onClose });
  const heading = entry === null ? t("sftp.detailsForCount", { count: entries.length }) : t("sftp.detailsFor", { name: entry.name });

  // Only a remote directory has its size counted; anything else reads nothing.
  const readDirectoryStats = useCallback(
    async (): Promise<RemoteDirectoryStats | null> => directoryPath === null ? null : sftpApi.directoryStats(alias, directoryPath),
    [alias, directoryPath],
  );
  const directoryStatsLoad = useLoad(readDirectoryStats);
  const directoryStats = directoryStatsLoad.state === "loaded" ? directoryStatsLoad.value : null;
  const calculating = directoryPath !== null && directoryStatsLoad.state === "loading";

  const properties = (
        <dl role="group" className="min-h-0 overflow-auto border-t border-line px-3 py-2 md:border-l md:border-t-0" aria-label={t("sftp.properties")}>
          {entry === null ? (
            <>
              <Property label={t("sftp.selectedItems")}>{entries.length.toLocaleString(locale)}</Property>
              <Property label={t("sftp.totalSize")}>{`${totalBytes.toLocaleString(locale)} (${formatBytes(totalBytes)})`}</Property>
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
                  ? `${entry.size.toLocaleString(locale)} (${formatBytes(entry.size)})`
                  : calculating
                    ? t("sftp.calculating")
                    : directoryStats !== null
                      ? `${directoryStats.truncated ? t("sftp.partialSizePrefix") : ""}${directoryStats.bytes.toLocaleString(locale)} (${formatBytes(directoryStats.bytes)})`
                      : t("sftp.sizeUnavailable")}
              </Property>
              {kind === "directory" && directoryStats !== null ? (
                <>
                  <Property label={t("sftp.containedFiles")}>{directoryStats.files.toLocaleString(locale)}</Property>
                  <Property label={t("sftp.containedDirectories")}>{Math.max(0, directoryStats.directories - 1).toLocaleString(locale)}</Property>
                </>
              ) : null}
              <Property label={t("sftp.modified")}>
                <time dateTime={entry.modifiedAt}>{formatDateTime(entry.modifiedAt, locale)}</time>
              </Property>
              <Property label={t("sftp.permissions")}>{`${entry.mode} (${symbolicModeToOctal(entry.mode)})`}</Property>
              <Property label={t("sftp.uid")}>{entry.uid ?? t("sftp.ownershipUnavailable")}</Property>
              <Property label={t("sftp.gid")}>{entry.gid ?? t("sftp.ownershipUnavailable")}</Property>
              <Property label={t("sftp.revision")}>{entry.revision}</Property>
            </>
          )}
        </dl>
  );

  // A picture or a recording is not edited as text, so only other files offer the editor.
  const pictured = preview.kind === "image" || preview.kind === "media";
  const downloadable = entries.some(canDownload);
  // What the entries offer besides the download. A PC lists them under the details, the editor first; a
  // phone keeps the screen for the preview and puts them in the header's "…", beside the download.
  const editAction = entry !== null && kind === "file" && !pictured ? [{ label: t("sftp.editFile"), disabled: busy, run: () => onEdit(entry) }] : [];
  const entryActions = [
    ...(entry?.type === "symlink" && onChangeLinkTarget ? [{ label: t("sftp.changeLinkTarget"), disabled: busy, run: () => onChangeLinkTarget(entry) }] : []),
    ...(entry !== null && (entry.type === "file" || entry.type === "directory") && onChangeOwnership
      ? [{ label: t("sftp.changeOwnership"), disabled: busy || entry.uid === undefined || entry.gid === undefined, run: () => onChangeOwnership(entry) }] : []),
    ...(entry === null ? [] : [{ label: t("sftp.rename"), disabled: busy, run: () => onRename(entry) }]),
  ];
  const otherActions = [...editAction, ...entryActions];

  return (
    <ModalShell
      labelledBy={headingId}
      onDismiss={onClose}
      {...(returnFocusRef === undefined ? {} : { returnFocusRef })}
      placement={mobileViewport ? "fullscreen" : "center"}
      panelClassName={mobileViewport
        ? "flex h-dvh w-full flex-col overflow-hidden rounded-none border-0! shadow-none"
        : "flex h-[min(46rem,calc(100dvh-2rem))] w-full max-w-4xl flex-col overflow-hidden rounded-lg"}
    >
      <div className="flex items-center gap-2 border-b border-line bg-toolbar px-3 py-2">
        <h2 id={headingId} className="min-w-0 grow truncate text-sm font-medium">{heading}</h2>
        <PreviewStepper position={stepPosition} count={stepCount} onStep={stepTo} />
        {mobileViewport && downloadable ? <PreviewIconButton icon="download" label={t("sftp.download")} disabled={busy} onClick={() => onDownload(entries)} /> : null}
        {mobileViewport && otherActions.length > 0 ? (
          <ActionMenu label={t("sftp.selectedActions", { name: entry?.name ?? heading })}
            items={otherActions.map(({ label, disabled, run }) => ({ label, disabled, onSelect: run }))}
            triggerClassName="flex size-11 shrink-0 items-center justify-center rounded text-ink-muted hover:bg-hover hover:text-ink" />
        ) : null}
        <button type="button" className="text-xs text-ink-muted hover:text-ink" onClick={onClose}>{t("sftp.close")}</button>
      </div>

      <div className={mobileViewport ? "flex min-h-0 flex-1 flex-col overflow-hidden" : "grid min-h-0 flex-1 grid-cols-[minmax(0,1fr)_18rem] overflow-hidden"}>
        <div role="group" className={`min-h-0 min-w-0 flex-1 overflow-auto overscroll-contain ${mobileViewport && pictured ? "bg-black" : "bg-surface-subtle p-3"}`} aria-label={t("sftp.preview")}>
          {entry === null ? (
            <ul className="space-y-1 text-xs" aria-label={t("sftp.selectedItems")}>
              {entries.map((item) => (
                <li key={item.path} className="break-all font-mono text-ink-muted">{item.path}</li>
              ))}
            </ul>
          ) : preview.kind !== "idle" ? (
            <FilePreviewBody preview={preview} path={entry.path} name={entry.name} onSwipe={onSwipe} />
          ) : (
            <p className="text-sm text-ink-muted">{t(entryTypeLabelKeys[entry.type])}</p>
          )}
        </div>

        {mobileViewport ? <details className="min-h-0 shrink-0 overflow-auto border-t border-line"><DisclosureSummary className="px-3 py-3 text-sm text-ink-muted">{t("sftp.properties")}</DisclosureSummary><div className="max-h-40 overflow-auto">{properties}</div></details> : properties}
      </div>

      {mobileViewport ? null : (
        <div className="flex flex-wrap justify-end gap-2 border-t border-line px-3 py-2">
          {editAction.map(({ label, disabled, run }) => <Button key={label} disabled={disabled} onClick={run}>{label}</Button>)}
          {downloadable ? <Button disabled={busy} onClick={() => onDownload(entries)}>{t("sftp.download")}</Button> : null}
          {entryActions.map(({ label, disabled, run }) => <Button key={label} disabled={disabled} onClick={run}>{label}</Button>)}
        </div>
      )}
    </ModalShell>
  );
}
