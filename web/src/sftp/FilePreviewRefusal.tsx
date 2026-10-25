import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";

// FilePreviewRefusal says why a file is not shown. onDownload, when given, is
// offered right under the reason: a file that cannot be shown here is still
// one click from being saved.
export function FilePreviewRefusal({ reason, onDownload }: { reason: MessageKey; onDownload?: (() => void) | undefined }) {
  const t = useTranslate();
  return (
    <div className="grid justify-items-start gap-3">
      <p className="text-sm text-ink-muted">{t(reason)}</p>
      {onDownload === undefined ? null : (
        <button type="button" className="min-h-11 rounded-md border border-control-line px-3 text-sm hover:bg-select-fill md:min-h-8" onClick={onDownload}>
          {t("sftp.download")}
        </button>
      )}
    </div>
  );
}
