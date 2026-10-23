import { useEffect, useId, useState } from "react";
import { useTranslate } from "../i18n/context";
import { ModalShell } from "../ui/ModalShell";
import { FilePreviewBody } from "./FilePreviewBody";
import { useFilePreview } from "./useFilePreview";

function baseName(path: string): string {
  const components = path.split("/").filter(Boolean);
  return components[components.length - 1] ?? path;
}

// FilePreviewDialog shows one file over the screen that named it, such as a
// path a terminal printed. The file is read when it is shown, never before,
// so a program printing many paths does not make sshc read them.
//
// paths are the files the arrow keys step through, in order. When path is
// not among them, the dialog shows path alone.
export function FilePreviewDialog({
  alias,
  path,
  paths = [],
  onClose,
}: {
  alias: string;
  path: string;
  paths?: string[];
  onClose: () => void;
}) {
  const t = useTranslate();
  const headingId = useId();
  const steps = paths.includes(path) ? paths : [path];
  const [position, setPosition] = useState(() => steps.indexOf(path));
  const shown = steps[position] ?? path;
  const preview = useFilePreview(alias, shown);
  const hasPrevious = position > 0;
  const hasNext = position < steps.length - 1;

  // The arrows are read from the whole document: a step button that just
  // became disabled gives up focus, and the keys must keep working after it.
  useEffect(() => {
    const step = (event: KeyboardEvent) => {
      if (event.key === "ArrowLeft" && hasPrevious) setPosition(position - 1);
      else if (event.key === "ArrowRight" && hasNext) setPosition(position + 1);
      else return;
      event.preventDefault();
    };
    document.addEventListener("keydown", step);
    return () => document.removeEventListener("keydown", step);
  }, [position, hasPrevious, hasNext]);

  return (
    <ModalShell
      labelledBy={headingId}
      onDismiss={onClose}
      closeOnOutside
      panelClassName="flex h-[min(46rem,calc(100dvh-2rem))] w-full max-w-4xl flex-col overflow-hidden rounded-lg"
    >
      <div className="flex items-center gap-2 border-b border-line bg-toolbar px-3 py-2">
        <h2 id={headingId} className="min-w-0 grow truncate font-mono text-sm font-medium" title={shown}>{shown}</h2>
        {steps.length > 1 ? (
          <>
            <button type="button" className="text-xs text-ink-muted hover:text-ink disabled:opacity-40" disabled={!hasPrevious} onClick={() => setPosition(position - 1)}>
              {t("sftp.previewPrevious")}
            </button>
            <span className="text-xs tabular-nums text-ink-muted">{t("sftp.previewPosition", { position: String(position + 1), count: String(steps.length) })}</span>
            <button type="button" className="text-xs text-ink-muted hover:text-ink disabled:opacity-40" disabled={!hasNext} onClick={() => setPosition(position + 1)}>
              {t("sftp.previewNext")}
            </button>
          </>
        ) : null}
        <button type="button" className="text-xs text-ink-muted hover:text-ink" onClick={onClose}>{t("sftp.close")}</button>
      </div>
      <div role="group" className="min-h-0 flex-1 overflow-auto overscroll-contain bg-surface-subtle p-3" aria-label={t("sftp.preview")}>
        <FilePreviewBody preview={preview} name={baseName(shown)} />
      </div>
    </ModalShell>
  );
}
