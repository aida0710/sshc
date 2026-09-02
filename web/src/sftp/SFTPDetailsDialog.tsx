import { useEffect, useId, useState, type ReactNode, type RefObject } from "react";
import { failureCode } from "../api/client";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { ModalShell } from "../ui/ModalShell";
import { Button } from "../ui/surface";
import { sftpApi, type RemoteEntry } from "./api";
import { formatBytes } from "./format";
import { symbolicModeToOctal } from "./transfers";

// Long text is previewed, not edited. Rendering a whole 2 MiB file into a <pre>
// costs more than the glance it is meant to give; the editor opens the rest.
const previewTextLimit = 64 << 10;

type PreviewState =
  | { kind: "idle" }
  | { kind: "loading" }
  | { kind: "image"; url: string }
  | { kind: "text"; contents: string; truncated: boolean }
  | { kind: "unavailable"; reason: MessageKey };

// A data: URL keeps the image inside the policy the app already ships;
// img-src allows data: and nothing here needs a Blob URL.
function dataURL(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(typeof reader.result === "string" ? reader.result : "");
    reader.onerror = () => reject(reader.error ?? new Error("preview_unreadable"));
    reader.readAsDataURL(blob);
  });
}

function unavailableReason(error: unknown): MessageKey {
  switch (failureCode(error)) {
    case "sftp_not_utf8":
    case "sftp_preview_type":
      return "sftp.binaryHint";
    case "sftp_text_too_large":
    case "sftp_preview_too_large":
      return "sftp.tooLargeHint";
    default:
      return "sftp.previewUnavailable";
  }
}

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
  onRename,
  returnFocusRef,
}: {
  alias: string;
  entries: RemoteEntry[];
  busy: boolean;
  onClose: () => void;
  onEdit: (entry: RemoteEntry) => void;
  onDownload: (entries: RemoteEntry[]) => void;
  onRename: (entry: RemoteEntry) => void;
  returnFocusRef?: RefObject<HTMLElement | null>;
}) {
  const t = useTranslate();
  const headingId = useId();
  const [preview, setPreview] = useState<PreviewState>({ kind: "idle" });
  const entry = entries.length === 1 ? entries[0] ?? null : null;
  const previewPath = entry !== null && entry.type === "file" ? entry.path : null;
  const totalBytes = entries.reduce((sum, item) => sum + (item.type === "file" ? item.size : 0), 0);
  const heading = entry === null ? t("sftp.detailsForCount", { count: entries.length }) : t("sftp.detailsFor", { name: entry.name });

  useEffect(() => {
    if (previewPath === null) {
      setPreview({ kind: "idle" });
      return;
    }
    let active = true;
    setPreview({ kind: "loading" });
    void (async () => {
      try {
        const previewed = await sftpApi.previewFile(alias, previewPath);
        const url = await dataURL(previewed.blob);
        if (!active) return;
        setPreview({ kind: "image", url });
        return;
      } catch (error) {
        if (!active) return;
        // Anything but "these bytes are not an image or a PDF" is final. Only
        // that answer is worth spending a second request on text.
        if (failureCode(error) !== "sftp_preview_type") {
          setPreview({ kind: "unavailable", reason: unavailableReason(error) });
          return;
        }
      }
      try {
        const file = await sftpApi.readText(alias, previewPath);
        if (!active) return;
        setPreview({
          kind: "text",
          contents: file.contents.slice(0, previewTextLimit),
          truncated: file.contents.length > previewTextLimit,
        });
      } catch (error) {
        if (!active) return;
        setPreview({ kind: "unavailable", reason: unavailableReason(error) });
      }
    })();
    return () => { active = false; };
  }, [alias, previewPath]);

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

      <div className="grid min-h-0 flex-1 grid-rows-[minmax(0,1fr)_auto] overflow-hidden md:grid-cols-[minmax(0,1fr)_18rem] md:grid-rows-1">
        <div role="group" className="min-h-0 min-w-0 overflow-auto bg-surface-subtle p-3" aria-label={t("sftp.preview")}>
          {entry === null ? (
            <ul className="space-y-1 text-xs" aria-label={t("sftp.selectedItems")}>
              {entries.map((item) => (
                <li key={item.path} className="break-all font-mono text-ink-muted">{item.path}</li>
              ))}
            </ul>
          ) : preview.kind === "loading" ? (
            <p className="text-sm text-ink-muted">{t("sftp.previewLoading")}</p>
          ) : preview.kind === "image" ? (
            <img src={preview.url} alt={entry.name} className="mx-auto max-h-full max-w-full object-contain" />
          ) : preview.kind === "text" ? (
            <>
              <pre className="whitespace-pre-wrap break-all font-mono text-xs text-ink">{preview.contents}</pre>
              {preview.truncated ? <p className="mt-2 text-xs text-ink-muted">{t("sftp.previewTruncated")}</p> : null}
            </>
          ) : preview.kind === "unavailable" ? (
            <p className="text-sm text-ink-muted">{t(preview.reason)}</p>
          ) : (
            <p className="text-sm text-ink-muted">{t(`sftp.type.${entry.type}`)}</p>
          )}
        </div>

        <dl role="group" className="min-h-0 overflow-auto border-t border-line px-3 py-2 md:border-l md:border-t-0" aria-label={t("sftp.properties")}>
          {entry === null ? (
            <>
              <Property label={t("sftp.selectedItems")}>{entries.length.toLocaleString()}</Property>
              <Property label={t("sftp.totalSize")}>{`${totalBytes.toLocaleString()} (${formatBytes(totalBytes)})`}</Property>
            </>
          ) : (
            <>
              <Property label={t("sftp.path")}>{entry.path}</Property>
              <Property label={t("sftp.type")}>{t(`sftp.type.${entry.type}`)}</Property>
              <Property label={t("sftp.size")}>
                {entry.type === "file" ? `${entry.size.toLocaleString()} (${formatBytes(entry.size)})` : "—"}
              </Property>
              <Property label={t("sftp.modified")}>
                <time dateTime={entry.modifiedAt}>{new Date(entry.modifiedAt).toLocaleString()}</time>
              </Property>
              <Property label={t("sftp.permissions")}>{`${entry.mode} (${symbolicModeToOctal(entry.mode)})`}</Property>
              <Property label={t("sftp.revision")}>{entry.revision}</Property>
            </>
          )}
        </dl>
      </div>

      <div className="flex flex-wrap justify-end gap-2 border-t border-line px-3 py-2">
        {entry !== null && entry.type === "file"
          ? <Button disabled={busy} onClick={() => onEdit(entry)}>{t("sftp.editFile")}</Button>
          : null}
        {entries.some((item) => item.type === "file" || item.type === "directory")
          ? <Button disabled={busy} onClick={() => onDownload(entries)}>{t("sftp.download")}</Button>
          : null}
        {entry === null ? null : <Button disabled={busy} onClick={() => onRename(entry)}>{t("sftp.rename")}</Button>}
      </div>
    </ModalShell>
  );
}
