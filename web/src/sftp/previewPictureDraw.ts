import { imageTransform, videoViewBox, type PreviewZoom } from "./previewZoom";

type Size = { width: number; height: number };

// While a finger moves a picture in a file preview, and while a spring finishes the move, the picture is drawn by
// writing its style here, on the element itself, and not by a render of the preview for every move of the finger.
// The zoom the preview keeps (usePreviewZoom) is set once the picture rests, and draws the same.

// drawPicture draws a pose on the picture. An image grows and moves by its transform. A zoomed video shows part of
// its frame (object-view-box), held at its fitted size, so its own controls keep their place; a video drawn smaller
// than fitted or moved aside by a swipe goes by its transform.
export function drawPicture(target: HTMLElement, pose: PreviewZoom, size: Size) {
  if (target instanceof HTMLVideoElement && pose.scale > 1) {
    target.style.transform = "";
    target.style.width = `${size.width}px`;
    target.style.height = `${size.height}px`;
    target.style.setProperty("object-view-box", videoViewBox(pose, size));
    return;
  }
  target.style.transform = imageTransform(pose);
  if (target instanceof HTMLVideoElement) {
    target.style.width = "";
    target.style.height = "";
    target.style.removeProperty("object-view-box");
  }
}

// forgetDrawnPicture gives a fitted picture's style back to the preview. An image's transform is the preview's own
// and stays as drawn; a video's fitted style is none.
export function forgetDrawnPicture(target: HTMLElement) {
  if (target instanceof HTMLVideoElement) target.style.transform = "";
}

type Surroundings = { area: HTMLElement; chrome: HTMLElement[]; panel: HTMLElement | null; frame: HTMLElement | null };

// The parts of a full-screen preview (FilePreviewFrame) that give way to the screen under it as a picture is pulled
// down to close it: the black around the picture, the controls drawn over it (data-preview-chrome: the header, the zoom
// bar, the properties), and the preview's own window and backdrop. A preview that is not drawn over the whole screen
// keeps them.
function surroundingsOf(target: HTMLElement): Surroundings | null {
  const area = target.closest<HTMLElement>("[data-preview-area='overlay']");
  if (area === null) return null;
  const panel = area.closest<HTMLElement>(".sshc-modal-panel");
  return {
    area,
    chrome: panel === null ? [] : [...panel.querySelectorAll<HTMLElement>("[data-preview-chrome]")],
    panel,
    frame: area.closest<HTMLElement>(".sshc-modal-frame"),
  };
}

// drawClosing draws the preview as far toward closed as progress says (0 to 1): the black thins to show the screen
// under it, and the controls fade out sooner, at half way.
export function drawClosing(target: HTMLElement, progress: number) {
  const parts = surroundingsOf(target);
  if (parts === null) return;
  const shown = Math.min(1, Math.max(0, progress));
  parts.area.style.backgroundColor = `rgb(0 0 0 / ${1 - shown})`;
  const chromeOpacity = String(Math.max(0, 1 - shown * 2));
  parts.chrome.forEach((control) => {
    control.style.opacity = chromeOpacity;
  });
  if (parts.panel !== null) parts.panel.style.background = "transparent";
  if (parts.frame !== null) {
    parts.frame.style.background = "transparent";
    parts.frame.style.backdropFilter = "none";
  }
}

// forgetClosing gives the surroundings back to their stylesheet.
export function forgetClosing(target: HTMLElement) {
  const parts = surroundingsOf(target);
  if (parts === null) return;
  parts.area.style.backgroundColor = "";
  parts.chrome.forEach((control) => {
    control.style.opacity = "";
  });
  if (parts.panel !== null) parts.panel.style.background = "";
  if (parts.frame !== null) {
    parts.frame.style.background = "";
    parts.frame.style.backdropFilter = "";
  }
}
