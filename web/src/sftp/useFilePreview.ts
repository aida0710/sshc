import { useEffect, useState } from "react";
import { failureCode } from "../api/client";
import type { MessageKey } from "../i18n/messages";
import { sftpApi } from "./api";
import { blobDataURL } from "./blobDataURL";
import { playableMediaKind, type PlayableMediaKind } from "./playableMedia";

export type FilePreviewState =
  | { kind: "idle" }
  | { kind: "loading" }
  | { kind: "image"; url: string }
  | { kind: "text"; contents: string }
  | { kind: "media"; element: PlayableMediaKind; url: string }
  | { kind: "unavailable"; reason: MessageKey };

function unavailableReason(error: unknown): MessageKey {
  switch (failureCode(error)) {
    case "sftp_not_utf8":
    case "sftp_preview_type":
      return "sftp.previewUnsupported";
    case "sftp_text_too_large":
    case "sftp_preview_too_large":
      return "sftp.previewTooLarge";
    case "sftp_not_found":
      return "sftp.previewNotFound";
    default:
      return "sftp.previewUnavailable";
  }
}

type ShowPreview = (preview: FilePreviewState) => void;

// The player gets a ticket, which lives as long as the player may ask for more
// of the file. The returned function stops the preview and revokes it.
function startMediaPreview({ alias, path, element, show }: { alias: string; path: string; element: PlayableMediaKind; show: ShowPreview }): () => void {
  let active = true;
  let ticket: string | null = null;
  void sftpApi.issueMediaTicket(alias, path).then((issued) => {
    ticket = issued;
    if (!active) {
      void sftpApi.revokeMediaTicket(issued).catch(() => undefined);
      return;
    }
    show({ kind: "media", element, url: sftpApi.mediaURL(issued) });
  }, (error: unknown) => {
    if (active) show({ kind: "unavailable", reason: unavailableReason(error) });
  });
  return () => {
    active = false;
    if (ticket !== null) void sftpApi.revokeMediaTicket(ticket).catch(() => undefined);
  };
}

// Any other file is read as an image, or as text when the engine says the
// bytes are not an image. The returned function stops the preview.
function startFilePreview({ alias, path, show }: { alias: string; path: string; show: ShowPreview }): () => void {
  let active = true;
  void (async () => {
    try {
      const previewed = await sftpApi.previewFile(alias, path);
      const url = await blobDataURL(previewed.blob);
      if (active) show({ kind: "image", url });
      return;
    } catch (error) {
      if (!active) return;
      // Anything but "these bytes are not an image" is final. Only that
      // answer is worth spending a second request on text.
      if (failureCode(error) !== "sftp_preview_type") {
        show({ kind: "unavailable", reason: unavailableReason(error) });
        return;
      }
    }
    try {
      const file = await sftpApi.readText(alias, path);
      if (active) show({ kind: "text", contents: file.contents });
    } catch (error) {
      if (active) show({ kind: "unavailable", reason: unavailableReason(error) });
    }
  })();
  return () => { active = false; };
}

// ShownPreview is a preview together with the file it was read from.
type ShownPreview = { alias: string; path: string; preview: FilePreviewState };

// useFilePreview shows the file at path on alias: audio and video play, and
// other files are read as an image or as text. A null path reads nothing.
//
// A preview read for another file is never returned: on the render that moves to a new path, before the effect
// starts reading it, the file is loading. Otherwise the previous picture would show for a moment under the new
// name, and be replaced under a finger that had started to swipe it. A read that is stopped forgets what it
// showed, so that coming back to a file reads it again rather than showing a player whose ticket was revoked.
export function useFilePreview(alias: string, path: string | null): FilePreviewState {
  const [shown, setShown] = useState<ShownPreview | null>(null);

  useEffect(() => {
    if (path === null) return;
    const show = (preview: FilePreviewState) => setShown({ alias, path, preview });
    const element = playableMediaKind(path);
    const stop = element === null
      ? startFilePreview({ alias, path, show })
      : startMediaPreview({ alias, path, element, show });
    return () => {
      stop();
      setShown(null);
    };
  }, [alias, path]);

  if (path === null) return { kind: "idle" };
  if (shown === null || shown.alias !== alias || shown.path !== path) return { kind: "loading" };
  return shown.preview;
}
