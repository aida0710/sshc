import { useState } from "react";
import { keyboardOwnerProps } from "../ui/useDismissibleLayer";
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
  if (failed) return <FilePreviewRefusal reason="sftp.previewMediaUnsupported" onDownload={onDownload} />;
  return element === "audio"
    ? <audio {...keyboardOwnerProps} src={url} controls preload="metadata" aria-label={name} className="w-full" onError={() => setFailed(true)} />
    : <video {...keyboardOwnerProps} src={url} controls preload="metadata" aria-label={name} className="mx-auto max-h-full max-w-full" onError={() => setFailed(true)} />;
}
