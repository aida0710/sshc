import type { ReactNode, RefObject } from "react";
import { useTranslate } from "../i18n/context";
import { ModalShell } from "../ui/ModalShell";
import { mobileViewportQuery, useMediaQuery } from "../ui/useMediaQuery";
import type { FilePreviewState } from "./useFilePreview";

// useFilePreviewOverlay says whether a preview draws its header over a picture: a phone gives the preview the whole
// screen, and a picture then fills it on black, under the header, as the phone's photos show one. Text and refusals
// keep the header above them, where it covers nothing.
export function useFilePreviewOverlay(preview: FilePreviewState): boolean {
  const mobile = useMediaQuery(mobileViewportQuery);
  return mobile && (preview.kind === "image" || preview.kind === "media");
}

// FilePreviewFrame is the window every file preview is shown in, wherever the file was opened from.
export function FilePreviewFrame({ headingId, header, overlay, onDismiss, returnFocusRef, children }: {
  headingId: string;
  header: ReactNode;
  overlay: boolean;
  onDismiss: () => void;
  returnFocusRef?: RefObject<HTMLElement | null> | undefined;
  children: ReactNode;
}) {
  const mobile = useMediaQuery(mobileViewportQuery);
  return (
    <ModalShell
      labelledBy={headingId}
      onDismiss={onDismiss}
      closeOnOutside
      {...(returnFocusRef === undefined ? {} : { returnFocusRef })}
      placement={mobile ? "fullscreen" : "center"}
      panelClassName={mobile
        ? "relative flex h-dvh w-full flex-col overflow-hidden rounded-none! border-0! shadow-none"
        : "flex h-[min(46rem,calc(100dvh-2rem))] w-full max-w-4xl flex-col overflow-hidden"}
    >
      {overlay ? <div data-preview-chrome className="absolute inset-x-0 top-0 z-10 bg-linear-to-b from-black/85 from-70% to-transparent pb-7">{header}</div> : header}
      {children}
    </ModalShell>
  );
}

// previewPictureArea centres a picture or a video in the preview and holds it to the preview's size. The grid's one row
// and column are sized by the preview rather than by what they hold: an image's max-height against a row that grows
// with the image does nothing, so a tall picture would run off the top and the bottom.
export const previewPictureArea = "relative grid h-full grid-cols-[minmax(0,1fr)] grid-rows-[minmax(0,1fr)] place-items-center overflow-hidden";

// FilePreviewArea is the place the file itself is drawn in.
export function FilePreviewArea({ overlay, children }: { overlay: boolean; children: ReactNode }) {
  const t = useTranslate();
  return (
    <div role="group" aria-label={t("sftp.preview")} data-preview-area={overlay ? "overlay" : "inline"} className={`min-h-0 min-w-0 flex-1 overscroll-contain ${overlay ? "overflow-hidden bg-black" : "overflow-auto bg-surface-subtle p-3"}`}>
      {children}
    </div>
  );
}
