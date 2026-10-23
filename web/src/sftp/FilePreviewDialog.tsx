import { useId } from "react";
import { useTranslate } from "../i18n/context";
import { ModalShell } from "../ui/ModalShell";
import { FilePreviewBody } from "./FilePreviewBody";
import { useFilePreview } from "./useFilePreview";

function baseName(path: string): string {
  const components = path.split("/").filter(Boolean);
  return components[components.length - 1] ?? path;
}

// FilePreviewDialog shows one file over the screen that named it, such as a
// path a terminal printed. The file is read when the dialog opens, never
// before, so a program printing many paths does not make sshc read them.
export function FilePreviewDialog({ alias, path, onClose }: { alias: string; path: string; onClose: () => void }) {
  const t = useTranslate();
  const headingId = useId();
  const preview = useFilePreview(alias, path);

  return (
    <ModalShell
      labelledBy={headingId}
      onDismiss={onClose}
      closeOnOutside
      panelClassName="flex h-[min(46rem,calc(100dvh-2rem))] w-full max-w-4xl flex-col overflow-hidden rounded-lg"
    >
      <div className="flex items-center gap-2 border-b border-line bg-toolbar px-3 py-2">
        <h2 id={headingId} className="min-w-0 grow truncate font-mono text-sm font-medium" title={path}>{path}</h2>
        <button type="button" className="text-xs text-ink-muted hover:text-ink" onClick={onClose}>{t("sftp.close")}</button>
      </div>
      <div role="group" className="min-h-0 flex-1 overflow-auto overscroll-contain bg-surface-subtle p-3" aria-label={t("sftp.preview")}>
        <FilePreviewBody preview={preview} name={baseName(path)} />
      </div>
    </ModalShell>
  );
}
