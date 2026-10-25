import { useState } from "react";
import { keyboardOwnerProps } from "../ui/useDismissibleLayer";
import { FilePreviewLoading } from "./FilePreviewLoading";
import { FilePreviewRefusal } from "./FilePreviewRefusal";
import type { PlayableMediaKind } from "./playableMedia";

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
  return (
    <div className="relative grid h-full place-items-center" aria-busy={waiting}>
      {element === "audio"
        ? <audio {...keyboardOwnerProps} {...playerEvents} onLoadedMetadata={() => setWaiting(false)} src={url} controls preload="metadata" aria-label={name} className="w-full self-start" />
        : <video {...keyboardOwnerProps} {...playerEvents} src={url} controls preload="metadata" aria-label={name} className="max-h-full max-w-full" />}
      {waiting ? <div className="pointer-events-none absolute inset-0"><FilePreviewLoading /></div> : null}
    </div>
  );
}
