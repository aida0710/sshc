import { rubberBand, rubberBandSlope } from "../ui/rubberBand";

// How far an image or a video in a file preview is zoomed and moved. scale 1 is the whole picture fitted to
// the preview; x and y move the zoomed picture, in pixels of the fitted picture, from its centre.
export type PreviewZoom = { scale: number; x: number; y: number };

export const fittedZoom: PreviewZoom = { scale: 1, x: 0, y: 0 };

// maxPreviewScale is eight times the fitted size: enough to read the pixels of a screenshot fitted into a
// phone, and no further, where only the blur grows.
export const maxPreviewScale = 8;

// previewZoomStep is one press of the zoom buttons: four presses double the size and back.
export const previewZoomStep = Math.SQRT2 ** 0.5;

type Point = { x: number; y: number };
type Size = { width: number; height: number };

function clamp(value: number, low: number, high: number): number {
  return Math.min(high, Math.max(low, value));
}

// keptInside stops the picture at its edges: a zoomed picture never shows the background past one.
function keptInside(zoom: PreviewZoom, size: Size): PreviewZoom {
  const reachX = ((zoom.scale - 1) * size.width) / 2;
  const reachY = ((zoom.scale - 1) * size.height) / 2;
  // `|| 0` turns the -0 a fitted picture can end with into 0.
  return { scale: zoom.scale, x: clamp(zoom.x, -reachX, reachX) || 0, y: clamp(zoom.y, -reachY, reachY) || 0 };
}

// zoomedAround scales by factor and keeps the spot under point where it is. point is measured from the
// fitted picture's centre, as x and y are, and size is the fitted picture's.
export function zoomedAround(zoom: PreviewZoom, factor: number, point: Point, size: Size): PreviewZoom {
  const scale = clamp(zoom.scale * factor, 1, maxPreviewScale);
  const kept = scale / zoom.scale;
  return keptInside({ scale, x: point.x - (point.x - zoom.x) * kept, y: point.y - (point.y - zoom.y) * kept }, size);
}

export function movedBy(zoom: PreviewZoom, dx: number, dy: number, size: Size): PreviewZoom {
  return keptInside({ scale: zoom.scale, x: zoom.x + dx, y: zoom.y + dy }, size);
}

// freeZoomedAround is zoomedAround without its limits, for a pinch in progress: the fingers may take the picture past
// fitted or past the largest size for a while, and stretchedZoom draws how far.
export function freeZoomedAround(zoom: PreviewZoom, factor: number, point: Point): PreviewZoom {
  return { scale: zoom.scale * factor, x: point.x - (point.x - zoom.x) * factor, y: point.y - (point.y - zoom.y) * factor };
}

// freeMovedBy is movedBy without stopping at the edges, for a finger dragging the picture past one.
export function freeMovedBy(zoom: PreviewZoom, dx: number, dy: number): PreviewZoom {
  return { scale: zoom.scale, x: zoom.x + dx, y: zoom.y + dy };
}

// Past its limits a scale gives as a band does (rubberBand), measured in hundredths, as the place is in pixels.
const scaleUnits = 100;

function stretchedPast(value: number, low: number, high: number, give: (overflow: number) => number): number {
  if (value < low) return low + give(value - low);
  if (value > high) return high + give(value - high);
  return value;
}

// stretchedZoom is how a pose a finger holds past the limits is drawn: smaller than fitted, larger than the largest
// size, or moved past an edge, it goes less and less far the further the finger pulls (rubberBand).
export function stretchedZoom(zoom: PreviewZoom, size: Size): PreviewZoom {
  const { scale, kept, reachX, reachY } = stretchedFrame(zoom, size);
  return {
    scale,
    x: stretchedPast(zoom.x * kept, -reachX, reachX, (overflow) => rubberBand(overflow, size.width)),
    y: stretchedPast(zoom.y * kept, -reachY, reachY, (overflow) => rubberBand(overflow, size.height)),
  };
}

// stretchedFollow is how far a picture drawn by stretchedZoom moves for each pixel a finger moves it, across and down:
// all of it within the edges, and less past them, where it gives only a little.
export function stretchedFollow(zoom: PreviewZoom, size: Size): Point {
  const { kept, reachX, reachY } = stretchedFrame(zoom, size);
  return {
    x: followPast(zoom.x * kept, -reachX, reachX, size.width),
    y: followPast(zoom.y * kept, -reachY, reachY, size.height),
  };
}

// stretchedFrame is the scale stretchedZoom draws a pose at, given past the limits, and how far that scale lets the
// picture move each way.
function stretchedFrame(zoom: PreviewZoom, size: Size) {
  const scale = stretchedPast(zoom.scale, 1, maxPreviewScale, (overflow) => rubberBand(overflow * scaleUnits, scaleUnits) / scaleUnits);
  return {
    scale,
    kept: scale / zoom.scale,
    reachX: (Math.max(0, scale - 1) * size.width) / 2,
    reachY: (Math.max(0, scale - 1) * size.height) / 2,
  };
}

function followPast(value: number, low: number, high: number, dimension: number): number {
  if (value < low) return rubberBandSlope(value - low, dimension);
  if (value > high) return rubberBandSlope(value - high, dimension);
  return 1;
}

// restingZoom is where a pose comes to rest once let go: its scale within the limits, around the same spot, and the
// picture kept inside its edges.
export function restingZoom(zoom: PreviewZoom, size: Size): PreviewZoom {
  const scale = clamp(zoom.scale, 1, maxPreviewScale);
  const kept = scale / zoom.scale;
  return keptInside({ scale, x: zoom.x * kept, y: zoom.y * kept }, size);
}

// imageTransform draws the zoom on an image, which grows around its centre.
export function imageTransform(zoom: PreviewZoom): string {
  return `translate(${zoom.x}px, ${zoom.y}px) scale(${zoom.scale})`;
}

// videoViewBox draws the zoom on a video as the part of the frame it shows (object-view-box). The video keeps
// its size on the page, so the browser's own controls stay where they are and as large as they were.
export function videoViewBox(zoom: PreviewZoom, size: Size): string {
  if (zoom.scale === 1 || size.width === 0 || size.height === 0) return "none";
  const shown = 100 / zoom.scale;
  const left = 50 - shown / 2 - (zoom.x / zoom.scale / size.width) * 100;
  const top = 50 - shown / 2 - (zoom.y / zoom.scale / size.height) * 100;
  const percent = (value: number) => `${Math.max(0, value).toFixed(3)}%`;
  return `inset(${percent(top)} ${percent(100 - shown - left)} ${percent(100 - shown - top)} ${percent(left)})`;
}
