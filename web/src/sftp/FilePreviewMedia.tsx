import { useState } from "react";
import { keyboardOwnerProps } from "../ui/useDismissibleLayer";
import { FilePreviewLoading } from "./FilePreviewLoading";
import { FilePreviewRefusal } from "./FilePreviewRefusal";
import type { PlayableMediaKind } from "./playableMedia";
import { videoViewBox } from "./previewZoom";
import { PreviewZoomBar } from "./PreviewZoomBar";
import { usePreviewZoom } from "./usePreviewZoom";

// videoControlsHeight is the height of the browser's own controls at the bottom of a video. A press there
// plays and seeks rather than moving a zoomed video.
const videoControlsHeight = 48;

// A video zooms by showing part of its frame (object-view-box), which keeps the browser's own controls as
// they are. A browser without it leaves the video unzoomed.
const videosZoom = typeof CSS !== "undefined" && typeof CSS.supports === "function" && CSS.supports("object-view-box", "inset(0%)");

// FilePreviewMedia plays audio or video with the browser's own controls. The
// engine sends only the ranges the player asks for, so seeking in a long
// recording does not wait for the whole file. A format the browser cannot
// decode (HEVC in a .mov, for example) falls back to the download. The
// player owns the arrow keys while it has focus, to seek, as the editor does.
export function FilePreviewMedia({ element, url, name, onDownload }: {
  element: PlayableMediaKind;
  url: string;
  name: string;
  onDownload?: (() => void) | undefined;
}) {
  const [failed, setFailed] = useState(false);
  // waiting is true until the first frame (or the length of the audio) has
  // arrived, and again while the player waits for the range it seeked to.
  // With preload="metadata" some browsers never fire canplay before play is
  // pressed, so the first frame and the audio's length also end the wait.
  const [waiting, setWaiting] = useState(true);
  const { target, zoom, size, areaProps, zoomIn, zoomOut, fit } = usePreviewZoom<HTMLVideoElement>({ keepBottom: videoControlsHeight });
  if (failed) return <FilePreviewRefusal reason="sftp.previewMediaUnsupported" onDownload={onDownload} />;
  const playerEvents = {
    onError: () => setFailed(true),
    onWaiting: () => setWaiting(true),
    onSeeking: () => setWaiting(true),
    onCanPlay: () => setWaiting(false),
    onLoadedData: () => setWaiting(false),
    onSeeked: () => setWaiting(false),
    onPlaying: () => setWaiting(false),
  };
  if (element === "audio") {
    return (
      <div className="relative grid h-full place-items-center" aria-busy={waiting}>
        <audio {...keyboardOwnerProps} {...playerEvents} onLoadedMetadata={() => setWaiting(false)} src={url} controls preload="metadata" aria-label={name} className="w-full self-start" />
        {waiting ? <div className="pointer-events-none absolute inset-0"><FilePreviewLoading /></div> : null}
      </div>
    );
  }
  return (
    <div {...(videosZoom ? areaProps : {})} data-preview-zoom-area={videosZoom ? "" : undefined}
      className={`relative grid h-full place-items-center overflow-hidden ${videosZoom ? "touch-none" : ""} ${zoom.scale > 1 ? "cursor-grab active:cursor-grabbing" : ""}`}
      aria-busy={waiting}>
      <video ref={target} {...keyboardOwnerProps} {...playerEvents} src={url} controls preload="metadata" aria-label={name}
        className="max-h-full max-w-full"
        // The part shown becomes the video's natural size, so a zoomed video is held at its fitted size, which
        // the shown part then fills.
        style={zoom.scale === 1 ? undefined : { width: size.width, height: size.height, objectViewBox: videoViewBox(zoom, size) }} />
      {videosZoom ? <PreviewZoomBar scale={zoom.scale} placement="top" onZoomIn={zoomIn} onZoomOut={zoomOut} onFit={fit} /> : null}
      {waiting ? <div className="pointer-events-none absolute inset-0"><FilePreviewLoading /></div> : null}
    </div>
  );
}
