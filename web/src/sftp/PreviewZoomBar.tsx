import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { maxPreviewScale } from "./previewZoom";

const button = "flex size-8 items-center justify-center rounded-full text-ink-muted hover:bg-hover hover:text-ink disabled:opacity-40 disabled:hover:bg-transparent";

// PreviewZoomBar floats over a picture in a file preview: zoom out, how far it is zoomed (pressing it fits
// the picture again), and zoom in. It sits at the top of a video, whose own controls take the bottom.
export function PreviewZoomBar({ scale, placement, onZoomIn, onZoomOut, onFit }: {
  scale: number;
  placement: "top" | "bottom";
  onZoomIn: () => void;
  onZoomOut: () => void;
  onFit: () => void;
}) {
  const t = useTranslate();
  const percent = `${Math.round(scale * 100)}%`;
  return (
    // A press on the bar is not the start of a drag on the picture under it.
    <div role="group" aria-label={t("sftp.previewZoom")} onPointerDown={(event) => event.stopPropagation()} onDoubleClick={(event) => event.stopPropagation()}
      className={`absolute right-2 z-10 flex items-center rounded-full border border-line bg-card/90 p-0.5 shadow-pane ${placement === "top" ? "top-2" : "bottom-2"}`}>
      <button type="button" className={button} aria-label={t("sftp.previewZoomOut")} title={t("sftp.previewZoomOut")} disabled={scale <= 1} onClick={onZoomOut}>
        <Icon name="zoomOut" className="size-4" />
      </button>
      <button type="button" className="h-8 min-w-12 rounded-full px-1 text-xs tabular-nums text-ink-muted hover:bg-hover hover:text-ink disabled:hover:bg-transparent"
        aria-label={t("sftp.previewZoomFit", { percent })} title={t("sftp.previewZoomFit", { percent })} disabled={scale === 1} onClick={onFit}>
        {percent}
      </button>
      <button type="button" className={button} aria-label={t("sftp.previewZoomIn")} title={t("sftp.previewZoomIn")} disabled={scale >= maxPreviewScale} onClick={onZoomIn}>
        <Icon name="zoomIn" className="size-4" />
      </button>
    </div>
  );
}
