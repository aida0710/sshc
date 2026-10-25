import { Suspense, lazy } from "react";
import { useTranslate } from "../i18n/context";
import { loadMonacoEditor } from "./loadMonacoEditor";
import type { FilePreviewState } from "./useFilePreview";

const MonacoEditor = lazy(() => loadMonacoEditor().then(({ MonacoEditor }) => ({ default: MonacoEditor })));

function ignoreEdit() {}

// FilePreviewBody draws a preview that is loading, loaded or refused. An idle
// preview draws nothing; the caller knows what to show before one is asked.
//
// Text is shown in the editor, read-only, so source files get the same
// colours as when they are edited. Markdown stays source: rendering it would
// need a renderer this preview does not otherwise need.
//
// onDownload, when given, is offered under a refusal: a file that cannot be
// shown here is still one click from being saved.
export function FilePreviewBody({ preview, path, name, onDownload }: {
  preview: FilePreviewState;
  path: string;
  name: string;
  onDownload?: () => void;
}) {
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
        <Suspense fallback={<p className="text-sm text-ink-muted">{t("sftp.editorLoading")}</p>}>
          <MonacoEditor path={path} value={preview.contents} onChange={ignoreEdit} readOnly />
        </Suspense>
      );
    case "unavailable":
      return (
        <div className="grid justify-items-start gap-3">
          <p className="text-sm text-ink-muted">{t(preview.reason)}</p>
          {onDownload === undefined ? null : (
            <button type="button" className="min-h-11 rounded-md border border-control-line px-3 text-sm hover:bg-select-fill md:min-h-8" onClick={onDownload}>
              {t("sftp.download")}
            </button>
          )}
        </div>
      );
  }
}
