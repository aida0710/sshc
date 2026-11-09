import { previewPictureArea } from "./FilePreviewFrame";
import type { PreviewSwipeHandler } from "./previewSwipe";
import { imageTransform } from "./previewZoom";
import { PreviewZoomBar } from "./PreviewZoomBar";
import { usePreviewZoom } from "./usePreviewZoom";

// FilePreviewImage shows an image fitted to the preview, and zoomed with the wheel, a pinch, a double-click
// or the zoom bar. A zoomed image is moved by dragging it; a fitted one is swiped (swipes).
export function FilePreviewImage({ url, name, swipes }: { url: string; name: string; swipes?: PreviewSwipeHandler | undefined }) {
  const { target, zoom, areaProps, zoomIn, zoomOut, fit } = usePreviewZoom<HTMLImageElement>({ doubleClickZooms: true, swipes });
  return (
    <div {...areaProps} data-preview-zoom-area
      className={`${previewPictureArea} touch-none select-none ${zoom.scale > 1 ? "cursor-grab active:cursor-grabbing" : "cursor-zoom-in"}`}>
      <img ref={target} src={url} alt={name} draggable={false}
        className="max-h-full max-w-full object-contain"
        style={{ transform: imageTransform(zoom) }} />
      <PreviewZoomBar scale={zoom.scale} placement="bottom" onZoomIn={zoomIn} onZoomOut={zoomOut} onFit={fit} />
    </div>
  );
}
