import { useTranslate } from "../i18n/context";
import type { FilePreviewState } from "./useFilePreview";

// FilePreviewBody draws a preview that is loading, loaded or refused. An idle
// preview draws nothing; the caller knows what to show before one is asked.
export function FilePreviewBody({ preview, name }: { preview: FilePreviewState; name: string }) {
  const t = useTranslate();
  switch (preview.kind) {
    case "idle":
      return null;
    case "loading":
      return <p className="text-sm text-ink-muted">{t("sftp.previewLoading")}</p>;
    case "image":
      return <img src={preview.url} alt={name} className="mx-auto max-h-full max-w-full object-contain" />;
    case "text":
      return (
        <>
          <pre className="whitespace-pre-wrap break-all font-mono text-xs text-ink">{preview.contents}</pre>
          {preview.truncated ? <p className="mt-2 text-xs text-ink-muted">{t("sftp.previewTruncated")}</p> : null}
        </>
      );
    case "unavailable":
      return <p className="text-sm text-ink-muted">{t(preview.reason)}</p>;
  }
}
