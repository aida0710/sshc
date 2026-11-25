import { useTranslate } from "../i18n/context";
import { PanelState } from "../ui/PanelState";

// FilePreviewLoading says the preview is still coming, centred in the space it
// will fill: reading a large file or a video over SSH can take a while, and a
// blank area looks like nothing will appear. It is the same answer as any panel
// still being read (PanelState).
export function FilePreviewLoading() {
  const t = useTranslate();
  return <PanelState tone="loading" title={t("sftp.previewLoading")} className="h-full" />;
}
