import { Suspense, lazy } from "react";
import { FilePreviewLoading } from "./FilePreviewLoading";
import { FilePreviewMedia } from "./FilePreviewMedia";
import { FilePreviewRefusal } from "./FilePreviewRefusal";
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
// onDownload, when given, is offered under a refusal (FilePreviewRefusal).
export function FilePreviewBody({ preview, path, name, onDownload }: {
  preview: FilePreviewState;
  path: string;
  name: string;
  onDownload?: () => void;
}) {
  switch (preview.kind) {
    case "idle":
      return null;
    case "loading":
      return <FilePreviewLoading />;
    case "image":
      return <img src={preview.url} alt={name} className="mx-auto max-h-full max-w-full object-contain" />;
    case "text":
      return (
        <Suspense fallback={<FilePreviewLoading />}>
          <MonacoEditor path={path} value={preview.contents} onChange={ignoreEdit} readOnly />
        </Suspense>
      );
    case "media":
      return <FilePreviewMedia key={preview.url} element={preview.element} url={preview.url} name={name} onDownload={onDownload} />;
    case "unavailable":
      return <FilePreviewRefusal reason={preview.reason} onDownload={onDownload} />;
  }
}
