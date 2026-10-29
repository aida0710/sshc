import { imageTransform } from "./previewZoom";
import { PreviewZoomBar } from "./PreviewZoomBar";
import { usePreviewZoom } from "./usePreviewZoom";

// FilePreviewImage shows an image fitted to the preview, and zoomed with the wheel, a pinch, a double-click
// or the zoom bar. A zoomed image is moved by dragging it.
export function FilePreviewImage({ url, name }: { url: string; name: string }) {
  const { target, zoom, areaProps, zoomIn, zoomOut, fit } = usePreviewZoom<HTMLImageElement>({ doubleClickZooms: true });
  return (
    <div {...areaProps} data-preview-zoom-area
      className={`relative grid h-full touch-none select-none place-items-center overflow-hidden ${zoom.scale > 1 ? "cursor-grab active:cursor-grabbing" : "cursor-zoom-in"}`}>
      <img ref={target} src={url} alt={name} draggable={false}
        className="max-h-full max-w-full object-contain"
        style={{ transform: imageTransform(zoom) }} />
      <PreviewZoomBar scale={zoom.scale} placement="bottom" onZoomIn={zoomIn} onZoomOut={zoomOut} onFit={fit} />
    </div>
  );
}
