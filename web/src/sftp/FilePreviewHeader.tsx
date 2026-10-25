import { useTranslate } from "../i18n/context";

// The controls never shrink: a squeezed button wraps its label one character
// per line. On a narrow screen the path takes the first row on its own and
// the controls move to the second.
const controlClassName = "min-h-11 shrink-0 whitespace-nowrap px-1 text-xs text-ink-muted hover:text-ink disabled:opacity-40 md:min-h-0";

// FilePreviewHeader names the shown file and holds the preview's controls:
// stepping through the other printed files, downloading, and closing.
export function FilePreviewHeader({
  headingId,
  path,
  position,
  count,
  onStep,
  onDownload,
  onClose,
}: {
  headingId: string;
  path: string;
  position: number;
  count: number;
  onStep: (position: number) => void;
  onDownload: () => void;
  onClose: () => void;
}) {
  const t = useTranslate();
  return (
    <div className="flex flex-wrap items-center gap-x-2 gap-y-1 border-b border-line bg-toolbar px-3 py-2">
      {/* A long path loses its start, not the file name: the right-to-left box
          puts the ellipsis on the left, and the bdi keeps the path itself in
          reading order. */}
      <h2 id={headingId} dir="rtl" className="min-w-0 basis-full truncate text-left font-mono text-sm font-medium md:basis-0 md:grow" title={path}>
        <bdi dir="ltr">{path}</bdi>
      </h2>
      <div className="ml-auto flex shrink-0 items-center gap-2">
        {count > 1 ? (
          <>
            <button type="button" className={controlClassName} disabled={position === 0} onClick={() => onStep(position - 1)}>
              {t("sftp.previewPrevious")}
            </button>
            <span className="shrink-0 whitespace-nowrap text-xs tabular-nums text-ink-muted">
              {t("sftp.previewPosition", { position: String(position + 1), count: String(count) })}
            </span>
            <button type="button" className={controlClassName} disabled={position === count - 1} onClick={() => onStep(position + 1)}>
              {t("sftp.previewNext")}
            </button>
          </>
        ) : null}
        <button type="button" className={controlClassName} onClick={onDownload}>{t("sftp.download")}</button>
        <button type="button" className={controlClassName} onClick={onClose}>{t("sftp.close")}</button>
      </div>
    </div>
  );
}
