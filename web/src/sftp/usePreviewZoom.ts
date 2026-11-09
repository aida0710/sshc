import { useCallback, useEffect, useRef, useState, type MouseEvent as ReactMouseEvent, type PointerEvent as ReactPointerEvent } from "react";
import { newVelocityTracker, type PointerVelocity, type VelocityTracker } from "../ui/pointerVelocity";
import { rubberBand, rubberBandSlope } from "../ui/rubberBand";
import { runSprings, type SpringsRun } from "../ui/spring";
import { drawClosing, drawPicture, forgetClosing, forgetDrawnPicture } from "./previewPictureDraw";
import { releasedSwipe, swipeAxisOf, type PreviewSwipe, type PreviewSwipeHandler, type SwipeAxis } from "./previewSwipe";
import { fittedZoom, freeMovedBy, freeZoomedAround, previewZoomStep, restingZoom, stretchedFollow, stretchedZoom, zoomedAround, type PreviewZoom } from "./previewZoom";

// wheelZoomRate turns the wheel's pixels into a zoom factor: one notch of a mouse wheel (100px) is about a
// press of the zoom buttons, and a trackpad's small deltas zoom smoothly.
const wheelZoomRate = 0.0017;

// dragThreshold is how far a pointer moves before a press counts as a drag, so a click that wobbles a pixel
// still reaches the video it was meant for.
const dragThreshold = 4;

// doubleClickScale is how far a double-click zooms a fitted image: twice the fitted size.
const doubleClickScale = 2;

// A zoomed picture flung by a finger carries on this long's worth of the speed it left with, slowing as it goes:
// about how far a page flung on a phone coasts.
const momentumMilliseconds = 325;

// Pulled down to close, the picture shrinks by up to this share over the first half of the preview's height, as a
// phone's photos do, and the preview around it gives way to the screen under it over the same distance.
const closingShrink = 0.15;
const closingTravelShare = 0.5;

// A picture swiped off the preview (aside to the next or previous file, or down to close it) aims this far past the
// edge, so it leaves the sight still moving, and the swipe finishes then.
const stepGap = 24;

type Point = { x: number; y: number };
type Size = { width: number; height: number };
type Box = Size & { left: number; top: number };

// Layout is where the picture and the preview around it are, measured when a gesture starts. A transform does not
// move the layout box, so the offsets are those of the fitted picture whatever the pose.
type Layout = { size: Size; centre: Point; area: Box };

// A Stroke is one finger moving a fitted picture to swipe it. base is where the picture was when the finger took it:
// fitted, or on its way somewhere, caught from a spring.
type Stroke = { pointerId: number; from: Point; base: Point; axis: SwipeAxis | null; offset: Point; tracker: VelocityTracker };

// A Drag is one pointer moving a zoomed picture.
type Drag = { pointerId: number; from: Point; base: PreviewZoom; moved: boolean; touch: boolean; tracker: VelocityTracker };

const still: PointerVelocity = { x: 0, y: 0 };

function sameZoom(left: PreviewZoom, right: PreviewZoom): boolean {
  return left.scale === right.scale && left.x === right.x && left.y === right.y;
}

function motionReduced(): boolean {
  return document.documentElement.dataset.motion === "reduce";
}

// pictureGone tells whether a picture drawn at pose has left the area on the side a swipe takes it to, with none of it
// in sight.
function pictureGone(swipe: PreviewSwipe, pose: PreviewZoom, measured: Layout): boolean {
  const halfWidth = (measured.size.width * pose.scale) / 2;
  const centreX = measured.centre.x + pose.x;
  if (swipe === "next") return centreX + halfWidth <= measured.area.left;
  if (swipe === "previous") return centreX - halfWidth >= measured.area.left + measured.area.width;
  return measured.centre.y + pose.y - (measured.size.height * pose.scale) / 2 >= measured.area.top + measured.area.height;
}

// usePreviewZoom holds the zoom of one image or video in a file preview and reads the gestures that change it, from
// area (the box around the picture): the wheel zooms around the pointer, a drag moves a zoomed picture, two fingers
// pinch, and a double-click on an image goes between fitted and twice the size. target is the picture itself; its size
// on the page is the fitted size the zoom is measured against.
//
// keepBottom is the height at the bottom of the picture that never starts a drag: where a video's own controls are, so
// pressing them still plays and seeks.
//
// A finger stroked across the fitted picture swipes it (swipes): the picture follows the finger, and let go it steps
// to the next or previous file or closes the preview, or goes back. While a finger moves the picture it follows the
// finger exactly and gives a little past its limits; let go, a spring finishes the move at the finger's speed, and a
// finger can catch it on the way. The zoom is set when the picture rests.
export function usePreviewZoom<Target extends HTMLElement>({ keepBottom = 0, doubleClickZooms = false, swipes }: {
  keepBottom?: number;
  doubleClickZooms?: boolean;
  swipes?: PreviewSwipeHandler | undefined;
} = {}) {
  const target = useRef<Target>(null);
  const area = useRef<HTMLDivElement>(null);
  const [zoom, setZoom] = useState<PreviewZoom>(fittedZoom);
  // size is the fitted picture's size when it was last measured, for drawing the zoom of a video.
  const [size, setSize] = useState<Size>({ width: 0, height: 0 });
  // drawn is the pose on the screen: the zoom, or where a finger or a spring has the picture now.
  const drawn = useRef<PreviewZoom>(zoom);
  // held is the pose two fingers hold, before stretchedZoom draws it within the limits.
  const held = useRef<PreviewZoom>(zoom);
  const layout = useRef<Layout | null>(null);
  const pointers = useRef(new Map<number, Point>());
  const dragged = useRef(false);
  const stroke = useRef<Stroke | null>(null);
  const drag = useRef<Drag | null>(null);
  const spring = useRef<SpringsRun | null>(null);

  useEffect(() => {
    drawn.current = zoom;
  }, [zoom]);
  useEffect(() => () => {
    spring.current?.stop();
  }, []);

  const measure = useCallback((): Layout | null => {
    const element = target.current;
    const box = area.current;
    if (element === null || box === null) return null;
    const outer = box.getBoundingClientRect();
    const fitted = { width: element.offsetWidth, height: element.offsetHeight };
    setSize((current) => current.width === fitted.width && current.height === fitted.height ? current : fitted);
    const centre = { x: outer.left + element.offsetLeft + fitted.width / 2, y: outer.top + element.offsetTop + fitted.height / 2 };
    const measured = { size: fitted, centre, area: { left: outer.left, top: outer.top, width: outer.width, height: outer.height } };
    layout.current = measured;
    return measured;
  }, []);

  const draw = useCallback((pose: PreviewZoom) => {
    const element = target.current;
    const measured = layout.current;
    drawn.current = pose;
    if (element !== null && measured !== null) drawPicture(element, pose, measured.size);
  }, []);

  // commit sets the zoom to where the picture rests, which draws it the same.
  const commit = useCallback((pose: PreviewZoom) => {
    drawn.current = pose;
    held.current = pose;
    setZoom(pose);
    const element = target.current;
    if (element !== null && sameZoom(pose, fittedZoom)) forgetDrawnPicture(element);
  }, []);

  // stopSpring stops a spring on its way and leaves the picture where it is drawn.
  const stopSpring = useCallback(() => {
    spring.current?.stop();
    spring.current = null;
  }, []);

  const springTo = useCallback((to: PreviewZoom, velocity: PointerVelocity, { onFrame, onRest }: { onFrame?: ((pose: PreviewZoom) => void) | undefined; onRest: () => void }) => {
    stopSpring();
    const from = drawn.current;
    spring.current = runSprings({
      from: [from.scale, from.x, from.y],
      to: [to.scale, to.x, to.y],
      velocity: [0, velocity.x, velocity.y],
      scale: [100, 1, 1],
      onFrame: ([scale = to.scale, x = to.x, y = to.y]) => {
        const pose = { scale, x, y };
        draw(pose);
        onFrame?.(pose);
      },
      onRest: () => {
        spring.current = null;
        onRest();
      },
      reducedMotion: motionReduced(),
    });
  }, [draw, stopSpring]);

  // The zoom buttons, the wheel and a double-click start from where the picture is, also when a spring has it.
  const zoomBy = useCallback((factor: number, at?: Point) => {
    const measured = measure();
    if (measured === null) return;
    stopSpring();
    const point = at === undefined ? { x: 0, y: 0 } : { x: at.x - measured.centre.x, y: at.y - measured.centre.y };
    commit(zoomedAround(restingZoom(drawn.current, measured.size), factor, point, measured.size));
  }, [commit, measure, stopSpring]);

  const fit = useCallback(() => {
    stopSpring();
    commit(fittedZoom);
  }, [commit, stopSpring]);

  useEffect(() => {
    const element = area.current;
    if (element === null) return;
    const wheel = (event: WheelEvent) => {
      event.preventDefault();
      zoomBy(Math.exp(-event.deltaY * wheelZoomRate), { x: event.clientX, y: event.clientY });
    };
    element.addEventListener("wheel", wheel, { passive: false });
    return () => element.removeEventListener("wheel", wheel);
  }, [zoomBy]);

  const allows = (swipe: PreviewSwipe) => swipes?.allows(swipe) === true;

  // closingProgress is how far a picture pulled down is toward closed, from 0 to 1.
  const closingProgress = (y: number, measured: Layout) => {
    const travel = measured.area.height * closingTravelShare;
    return travel > 0 ? Math.max(0, y) / travel : 0;
  };

  // swipeDraw is where a stroke draws the picture (pose): following the finger, and giving a little where the stroke
  // has no file to go to, or goes up. follow is how far the picture moves there for each pixel the finger moves.
  const swipeDraw = (axis: SwipeAxis, offset: Point, measured: Layout): { pose: PreviewZoom; follow: number } => {
    if (axis === "across") {
      const toward: PreviewSwipe = offset.x < 0 ? "next" : "previous";
      if (allows(toward)) return { pose: { scale: 1, x: offset.x, y: 0 }, follow: 1 };
      return { pose: { scale: 1, x: rubberBand(offset.x, measured.area.width), y: 0 }, follow: rubberBandSlope(offset.x, measured.area.width) };
    }
    if (offset.y < 0 || !allows("close")) {
      return { pose: { scale: 1, x: 0, y: rubberBand(offset.y, measured.area.height) }, follow: rubberBandSlope(offset.y, measured.area.height) };
    }
    return { pose: { scale: 1 - closingShrink * Math.min(1, closingProgress(offset.y, measured)), x: 0, y: offset.y }, follow: 1 };
  };

  // leave springs the picture off the area for a swipe, and swipes as soon as none of it is in sight: the spring's last
  // slow pixels are off the screen, and waiting for them would leave the preview empty for a moment.
  const leave = ({ swipe, to, velocity, measured, onFrame }: {
    swipe: PreviewSwipe;
    to: PreviewZoom;
    velocity: PointerVelocity;
    measured: Layout;
    onFrame?: ((pose: PreviewZoom) => void) | undefined;
  }) => {
    let swiped = false;
    const finish = () => {
      if (swiped) return;
      swiped = true;
      stopSpring();
      swipes?.swipe(swipe);
    };
    springTo(to, velocity, {
      onFrame: (pose) => {
        onFrame?.(pose);
        if (pictureGone(swipe, pose, measured)) finish();
      },
      onRest: finish,
    });
  };

  // releaseStroke finishes a stroke the finger let go of: a step aside, a close, or back to fitted.
  const releaseStroke = (finished: Stroke, velocity: PointerVelocity) => {
    const element = target.current;
    const measured = layout.current;
    if (element === null || measured === null || finished.axis === null) return;
    const swipe = releasedSwipe({ axis: finished.axis, offset: finished.offset, velocity, width: measured.area.width, allows });
    const closing = (pose: PreviewZoom) => drawClosing(element, closingProgress(pose.y, measured));
    // The picture leaves the finger at the speed it moved under it, along the stroke only.
    const { follow } = swipeDraw(finished.axis, finished.offset, measured);
    const moving = finished.axis === "across" ? { x: velocity.x * follow, y: 0 } : { x: 0, y: velocity.y * follow };
    if (swipe === "next" || swipe === "previous") {
      const aside = (swipe === "next" ? -1 : 1) * (measured.area.width + stepGap);
      leave({ swipe, to: { scale: 1, x: aside, y: 0 }, velocity: moving, measured });
      return;
    }
    if (swipe === "close") {
      leave({ swipe, to: { scale: 1 - closingShrink, x: 0, y: measured.area.height + stepGap }, velocity: moving, measured, onFrame: closing });
      return;
    }
    springTo(fittedZoom, moving, {
      onFrame: finished.axis === "down" ? closing : undefined,
      onRest: () => {
        forgetClosing(element);
        commit(fittedZoom);
      },
    });
  };

  // releaseDrag finishes a drag of a zoomed picture: a finger's fling carries it on and it rests inside its edges; a
  // mouse leaves it where it is, unless it was pulled past an edge.
  const releaseDrag = (finished: Drag, velocity: PointerVelocity) => {
    const measured = layout.current;
    if (measured === null) return;
    const carried = finished.touch ? velocity : still;
    const pose = held.current;
    const rest = restingZoom(freeMovedBy(pose, carried.x * momentumMilliseconds, carried.y * momentumMilliseconds), measured.size);
    // Past an edge the picture moved slower than the finger, and leaves it at that speed.
    const follow = stretchedFollow(pose, measured.size);
    springTo(rest, { x: carried.x * follow.x, y: carried.y * follow.y }, { onRest: () => commit(rest) });
  };

  // releasePinch finishes a pinch: the picture goes back within the limits, around the spot under the fingers.
  const releasePinch = () => {
    const measured = layout.current;
    if (measured === null) return;
    const rest = restingZoom(held.current, measured.size);
    springTo(rest, still, { onRest: () => commit(rest) });
  };

  function onPointerDown(event: ReactPointerEvent<HTMLDivElement>) {
    const measured = measure();
    if (measured === null || (event.pointerType === "mouse" && event.button !== 0)) return;
    if (keepBottom > 0 && event.clientY > measured.centre.y + measured.size.height / 2 - keepBottom) return;
    const point = { x: event.clientX, y: event.clientY };
    pointers.current.set(event.pointerId, point);
    dragged.current = false;
    // A finger on a picture still on its way takes it where it is.
    const caught = spring.current !== null;
    stopSpring();
    const pose = drawn.current;
    held.current = pose;
    // A second finger turns a swipe into a pinch; the preview around the picture comes back if it had begun to give way.
    const element = target.current;
    if (stroke.current?.axis === "down" && element !== null) forgetClosing(element);
    stroke.current = null;
    drag.current = null;
    if (pointers.current.size > 1) return;
    const tracker = newVelocityTracker(point.x, point.y, event.timeStamp);
    // A finger on a fitted picture swipes it, also on one a swipe or a close has on its way (drawn no larger).
    if (event.pointerType === "touch" && pose.scale <= 1) {
      const axis: SwipeAxis | null = pose.x !== 0 ? "across" : pose.y !== 0 ? "down" : null;
      stroke.current = { pointerId: event.pointerId, from: point, base: { x: pose.x, y: pose.y }, axis, offset: { x: pose.x, y: pose.y }, tracker };
      return;
    }
    drag.current = { pointerId: event.pointerId, from: point, base: pose, moved: caught, touch: event.pointerType === "touch", tracker };
  }

  // The pointer is captured once a drag or a pinch starts, not on the press: a captured press clicks the area
  // rather than what it was on, and a click on the video would no longer play it.
  function capture(event: ReactPointerEvent<HTMLDivElement>) {
    if (!event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.setPointerCapture(event.pointerId);
  }

  function onPointerMove(event: ReactPointerEvent<HTMLDivElement>) {
    const previous = pointers.current.get(event.pointerId);
    const measured = layout.current;
    if (previous === undefined || measured === null) return;
    const now = { x: event.clientX, y: event.clientY };
    pointers.current.set(event.pointerId, now);
    const others = [...pointers.current].filter(([id]) => id !== event.pointerId).map(([, point]) => point);
    if (others.length === 1) {
      // Two fingers: the gap between them sets the zoom and their midpoint stays under them.
      const other = others[0]!;
      const before = Math.hypot(previous.x - other.x, previous.y - other.y);
      const after = Math.hypot(now.x - other.x, now.y - other.y);
      if (before === 0) return;
      dragged.current = true;
      capture(event);
      const middle = { x: (now.x + other.x) / 2 - measured.centre.x, y: (now.y + other.y) / 2 - measured.centre.y };
      held.current = freeMovedBy(freeZoomedAround(held.current, after / before, middle), (now.x - previous.x) / 2, (now.y - previous.y) / 2);
      draw(stretchedZoom(held.current, measured.size));
      return;
    }
    const moving = stroke.current;
    if (moving !== null && moving.pointerId === event.pointerId) {
      moving.tracker.move(now.x, now.y, event.timeStamp);
      const moved = { x: now.x - moving.from.x, y: now.y - moving.from.y };
      moving.axis ??= swipeAxisOf(moved);
      if (moving.axis === null) return;
      dragged.current = true;
      capture(event);
      moving.offset = moving.axis === "across" ? { x: moving.base.x + moved.x, y: 0 } : { x: 0, y: moving.base.y + moved.y };
      const { pose } = swipeDraw(moving.axis, moving.offset, measured);
      draw(pose);
      const element = target.current;
      if (element !== null && moving.axis === "down") drawClosing(element, allows("close") ? closingProgress(pose.y, measured) : 0);
      return;
    }
    const dragging = drag.current;
    if (dragging === null || dragging.pointerId !== event.pointerId) return;
    dragging.tracker.move(now.x, now.y, event.timeStamp);
    if (!dragging.moved && Math.hypot(now.x - dragging.from.x, now.y - dragging.from.y) < dragThreshold) return;
    dragging.moved = true;
    dragged.current = true;
    capture(event);
    held.current = freeMovedBy(dragging.base, now.x - dragging.from.x, now.y - dragging.from.y);
    draw(stretchedZoom(held.current, measured.size));
  }

  function onPointerUp(event: ReactPointerEvent<HTMLDivElement>) {
    const wasPinch = pointers.current.size > 1;
    pointers.current.delete(event.pointerId);
    if (wasPinch) {
      if (pointers.current.size === 1) releasePinch();
      return;
    }
    const finished = stroke.current;
    if (finished !== null && finished.pointerId === event.pointerId) {
      stroke.current = null;
      releaseStroke(finished, finished.tracker.velocity(event.timeStamp));
      return;
    }
    const dragging = drag.current;
    if (dragging !== null && dragging.pointerId === event.pointerId) {
      drag.current = null;
      if (dragging.moved) releaseDrag(dragging, dragging.tracker.velocity(event.timeStamp));
    }
  }

  // A pointer the browser takes away (to scroll, or for a system gesture) lets the picture go back to rest.
  function onPointerCancel(event: ReactPointerEvent<HTMLDivElement>) {
    const wasPinch = pointers.current.size > 1;
    pointers.current.delete(event.pointerId);
    const finished = stroke.current;
    stroke.current = null;
    const dragging = drag.current;
    drag.current = null;
    if (wasPinch) releasePinch();
    else if (finished !== null && finished.axis !== null) releaseStroke({ ...finished, offset: { x: 0, y: 0 } }, still);
    else if (dragging !== null && dragging.moved) releaseDrag(dragging, still);
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
    if (restingZoom(drawn.current, layout.current?.size ?? size).scale === 1) zoomBy(doubleClickScale, { x: event.clientX, y: event.clientY });
    else fit();
  }

  return {
    target,
    zoom,
    size,
    areaProps: {
      ref: area,
      onPointerDown,
      onPointerMove,
      onPointerUp,
      onPointerCancel,
      onClickCapture,
      onDoubleClick,
    },
    zoomIn: () => zoomBy(previewZoomStep),
    zoomOut: () => zoomBy(1 / previewZoomStep),
    fit,
  };
}
