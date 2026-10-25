import { useTranslate } from "../i18n/context";
import { Spinner } from "../ui/Spinner";

// FilePreviewLoading says the preview is still coming, centred in the space it
// will fill: reading a large file or a video over SSH can take a while, and a
// blank area looks like nothing will appear.
export function FilePreviewLoading() {
  const t = useTranslate();
  return (
    <div role="status" className="flex h-full min-h-32 items-center justify-center gap-2 text-sm text-ink-muted">
      <Spinner className="size-4 text-accent" />
      {t("sftp.previewLoading")}
    </div>
  );
}
