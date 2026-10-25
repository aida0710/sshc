import { useEffect, useState } from "react";
import { failureCode } from "../api/client";
import type { MessageKey } from "../i18n/messages";
import { sftpApi } from "./api";

export type FilePreviewState =
  | { kind: "idle" }
  | { kind: "loading" }
  | { kind: "image"; url: string }
  | { kind: "text"; contents: string }
  | { kind: "unavailable"; reason: MessageKey };

// A data: URL keeps the image inside the policy the app already ships;
// img-src allows data: and nothing here needs a Blob URL.
function dataURL(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(typeof reader.result === "string" ? reader.result : "");
    reader.onerror = () => reject(reader.error ?? new Error("preview_unreadable"));
    reader.readAsDataURL(blob);
  });
}

function unavailableReason(error: unknown): MessageKey {
  switch (failureCode(error)) {
    case "sftp_not_utf8":
    case "sftp_preview_type":
      return "sftp.previewUnsupported";
    case "sftp_text_too_large":
    case "sftp_preview_too_large":
      return "sftp.previewTooLarge";
    default:
      return "sftp.previewUnavailable";
  }
}

// useFilePreview reads the file at path on alias as an image, or as text when
// the engine says the bytes are not an image. A null path reads nothing.
export function useFilePreview(alias: string, path: string | null): FilePreviewState {
  const [preview, setPreview] = useState<FilePreviewState>({ kind: "idle" });

  useEffect(() => {
    if (path === null) {
      setPreview({ kind: "idle" });
      return;
    }
    let active = true;
    setPreview({ kind: "loading" });
    void (async () => {
      try {
        const previewed = await sftpApi.previewFile(alias, path);
        const url = await dataURL(previewed.blob);
        if (!active) return;
        setPreview({ kind: "image", url });
        return;
      } catch (error) {
        if (!active) return;
        // Anything but "these bytes are not an image" is final. Only that
        // answer is worth spending a second request on text.
        if (failureCode(error) !== "sftp_preview_type") {
          setPreview({ kind: "unavailable", reason: unavailableReason(error) });
          return;
        }
      }
      try {
        const file = await sftpApi.readText(alias, path);
        if (!active) return;
        setPreview({ kind: "text", contents: file.contents });
      } catch (error) {
        if (!active) return;
        setPreview({ kind: "unavailable", reason: unavailableReason(error) });
      }
    })();
    return () => { active = false; };
  }, [alias, path]);

  return preview;
}
