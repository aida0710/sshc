import { useCallback, useEffect, useRef, useState, type MouseEvent as ReactMouseEvent, type PointerEvent as ReactPointerEvent } from "react";
import { fittedZoom, movedBy, previewZoomStep, zoomedAround, type PreviewZoom } from "./previewZoom";

// wheelZoomRate turns the wheel's pixels into a zoom factor: one notch of a mouse wheel (100px) is about a
// press of the zoom buttons, and a trackpad's small deltas zoom smoothly.
const wheelZoomRate = 0.0017;

// dragThreshold is how far a pointer moves before a press counts as a drag, so a click that wobbles a pixel
// still reaches the video it was meant for.
const dragThreshold = 4;

// doubleClickScale is how far a double-click zooms a fitted image: twice the fitted size.
const doubleClickScale = 2;

type Point = { x: number; y: number };
type Size = { width: number; height: number };

// usePreviewZoom holds the zoom of one image or video in a file preview and reads the gestures that change
// it, from area (the box around the picture): the wheel zooms around the pointer, a drag moves a zoomed
// picture, two fingers pinch, and a double-click on an image goes between fitted and twice the size.
// target is the picture itself; its size on the page is the fitted size the zoom is measured against.
//
// keepBottom is the height at the bottom of the picture that never starts a drag: where a video's own
// controls are, so pressing them still plays and seeks.
export function usePreviewZoom<Target extends HTMLElement>({ keepBottom = 0, doubleClickZooms = false }: {
  keepBottom?: number;
  doubleClickZooms?: boolean;
} = {}) {
  const target = useRef<Target>(null);
  const area = useRef<HTMLDivElement>(null);
  const [zoom, setZoom] = useState<PreviewZoom>(fittedZoom);
  // size is the fitted picture's size when it was last measured, for drawing the zoom of a video.
  const [size, setSize] = useState<Size>({ width: 0, height: 0 });
  const zoomRef = useRef(zoom);
  zoomRef.current = zoom;
  const pointers = useRef(new Map<number, Point>());
  const dragged = useRef(false);

  // The fitted picture's size and centre on the screen. A transform does not move the layout box, so the
  // offsets are those of the fitted picture whatever the zoom.
  const measure = useCallback(() => {
    const element = target.current;
    const box = area.current;
    if (element === null || box === null) return null;
    const outer = box.getBoundingClientRect();
    const fitted = { width: element.offsetWidth, height: element.offsetHeight };
    setSize((current) => current.width === fitted.width && current.height === fitted.height ? current : fitted);
    const centre = { x: outer.left + element.offsetLeft + fitted.width / 2, y: outer.top + element.offsetTop + fitted.height / 2 };
    return { size: fitted, centre };
  }, []);

  const zoomBy = useCallback((factor: number, at?: Point) => {
    const measured = measure();
    if (measured === null) return;
    const point = at === undefined ? { x: 0, y: 0 } : { x: at.x - measured.centre.x, y: at.y - measured.centre.y };
    setZoom((current) => zoomedAround(current, factor, point, measured.size));
  }, [measure]);

  // The wheel is read with a listener that may cancel it: React's is passive, and the page would scroll.
  useEffect(() => {
    const box = area.current;
    if (box === null) return;
    const wheel = (event: WheelEvent) => {
      event.preventDefault();
      zoomBy(Math.exp(-event.deltaY * wheelZoomRate), { x: event.clientX, y: event.clientY });
    };
    box.addEventListener("wheel", wheel, { passive: false });
    return () => box.removeEventListener("wheel", wheel);
  }, [zoomBy]);

  function onPointerDown(event: ReactPointerEvent<HTMLDivElement>) {
    const measured = measure();
    if (measured === null || (event.pointerType === "mouse" && event.button !== 0)) return;
    if (keepBottom > 0 && event.clientY > measured.centre.y + measured.size.height / 2 - keepBottom) return;
    pointers.current.set(event.pointerId, { x: event.clientX, y: event.clientY });
    dragged.current = false;
  }

  // The pointer is captured once a drag or a pinch starts, not on the press: a captured press clicks the area
  // rather than what it was on, and a click on the video would no longer play it.
  function capture(event: ReactPointerEvent<HTMLDivElement>) {
    if (!event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.setPointerCapture(event.pointerId);
  }

  function onPointerMove(event: ReactPointerEvent<HTMLDivElement>) {
    const previous = pointers.current.get(event.pointerId);
    const measured = measure();
    if (previous === undefined || measured === null) return;
    const others = [...pointers.current].filter(([id]) => id !== event.pointerId).map(([, point]) => point);
    const now = { x: event.clientX, y: event.clientY };
    pointers.current.set(event.pointerId, now);
    if (others.length === 1) {
      // Two fingers: the gap between them sets the zoom and their midpoint stays under them.
      const other = others[0]!;
      const before = Math.hypot(previous.x - other.x, previous.y - other.y);
      const after = Math.hypot(now.x - other.x, now.y - other.y);
      if (before === 0) return;
      dragged.current = true;
      capture(event);
      const middle = { x: (now.x + other.x) / 2 - measured.centre.x, y: (now.y + other.y) / 2 - measured.centre.y };
      setZoom((current) => movedBy(zoomedAround(current, after / before, middle, measured.size), (now.x - previous.x) / 2, (now.y - previous.y) / 2, measured.size));
      return;
    }
    if (zoomRef.current.scale === 1) return;
    if (!dragged.current && Math.hypot(now.x - previous.x, now.y - previous.y) < dragThreshold) {
      pointers.current.set(event.pointerId, previous);
      return;
    }
    dragged.current = true;
    capture(event);
    setZoom((current) => movedBy(current, now.x - previous.x, now.y - previous.y, measured.size));
  }

  function onPointerEnd(event: ReactPointerEvent<HTMLDivElement>) {
    pointers.current.delete(event.pointerId);
  }

  // A drag ends with a click on what was under it. A video would take it as play or pause.
  function onClickCapture(event: ReactMouseEvent<HTMLDivElement>) {
    if (!dragged.current) return;
    dragged.current = false;
    event.preventDefault();
    event.stopPropagation();
  }

  function onDoubleClick(event: ReactMouseEvent<HTMLDivElement>) {
    if (!doubleClickZooms) return;
    if (zoomRef.current.scale === 1) zoomBy(doubleClickScale, { x: event.clientX, y: event.clientY });
    else setZoom(fittedZoom);
  }

  return {
    target,
    zoom,
    size,
    areaProps: {
      ref: area,
      onPointerDown,
      onPointerMove,
      onPointerUp: onPointerEnd,
      onPointerCancel: onPointerEnd,
      onClickCapture,
      onDoubleClick,
    },
    zoomIn: () => zoomBy(previewZoomStep),
    zoomOut: () => zoomBy(1 / previewZoomStep),
    fit: () => setZoom(fittedZoom),
  };
}
