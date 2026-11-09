import { useEffect, useRef, type RefObject } from "react";
import { runSpring, type SpringRun } from "../ui/spring";

// How far a finger moves before the swipe is read as sideways or up and down. Below it a tap stays a tap.
const directionThreshold = 10;
// A swipe counts as sideways when it moves this many times further across than up or down, so scrolling a
// page with a slightly slanted finger does not pull the navigation out.
const sidewaysRatio = 1.2;
// Letting go past this share of the panel's width keeps it where the finger took it.
const settleShare = 0.35;
// A flick this fast (px per ms) settles it the same way, however short it was.
const flickSpeed = 0.35;
// A finger that rests this long before it moves is pressing long to select text, not swiping. Android's
// long press takes 400 ms.
const longPressMillis = 400;

// Where a swipe must not start: text fields select by dragging, and anything that scrolls sideways needs the
// sideways swipe for itself. The terminal scrolls only up and down, so a sideways swipe over it is the
// navigation's; its text is selected with a long press, which longPressMillis leaves alone.
const ownsTouchSelector = "input, textarea, select, [contenteditable='true']";

type Gesture = {
  // The panel the finger moves and the backdrop that darkens with it, taken when the finger goes down.
  panel: HTMLElement;
  backdrop: HTMLElement | null;
  opening: boolean;
  // Where the panel was when the finger took it: open, closed, or on its way, caught from the spring.
  base: number;
  // Where the panel is while it follows the finger, from 0 (open) to -width (closed).
  offset: number;
  startX: number;
  startY: number;
  startTime: number;
  dragging: boolean;
  width: number;
  lastX: number;
  lastTime: number;
  speed: number;
};

function scrollsSideways(element: Element): boolean {
  const overflow = getComputedStyle(element).overflowX;
  return (overflow === "auto" || overflow === "scroll") && element.scrollWidth > element.clientWidth + 1;
}

// A finger on selected text may be moving the selection's ends.
function onSelectedText(target: Element): boolean {
  const selection = target.ownerDocument.getSelection();
  if (selection === null || selection.isCollapsed) return false;
  for (let index = 0; index < selection.rangeCount; index += 1) {
    if (selection.getRangeAt(index).intersectsNode(target)) return true;
  }
  return false;
}

function ownsTheTouch(target: EventTarget | null): boolean {
  if (!(target instanceof Element)) return false;
  if (target.closest(ownsTouchSelector) !== null || onSelectedText(target)) return true;
  for (let element: Element | null = target; element !== null && element !== document.body; element = element.parentElement) {
    if (scrollsSideways(element)) return true;
  }
  return false;
}

// A dialog over the page takes every touch; swiping behind it must not move the navigation.
function dialogOpen(panel: HTMLElement): boolean {
  return Array.from(document.querySelectorAll("[aria-modal='true']")).some((dialog) => !panel.contains(dialog));
}

// While a finger drags the panel, and while the spring finishes the drag, its position is written inline:
// transform for where it is, and translate cleared so the closed state's -translate-x-full does not add to it.
// Once the panel rests where it settles, it is handed back to the stylesheet, which by then says the same.
function placePanel(panel: HTMLElement, offset: number) {
  panel.dataset.navigationDragging = "";
  panel.style.transition = "none";
  panel.style.translate = "none";
  panel.style.transform = `translateX(${offset}px)`;
}

// placeBackdrop darkens the page behind as far as the panel is out. Drawn from the panel's own place, the two
// cannot drift apart, also while the finger goes back and forth.
function placeBackdrop(backdrop: HTMLElement | null, offset: number, width: number) {
  if (backdrop === null) return;
  backdrop.style.transition = "none";
  backdrop.style.opacity = String(Math.min(1, Math.max(0, 1 + offset / width)));
}

// handBack gives the panel's and the backdrop's places back to the stylesheet, which draws them from the open state.
function handBack(panel: HTMLElement, backdrop: HTMLElement | null) {
  delete panel.dataset.navigationDragging;
  panel.style.transition = "";
  panel.style.transform = "";
  panel.style.translate = "";
  if (backdrop === null) return;
  backdrop.style.transition = "";
  backdrop.style.opacity = "";
}

// settlePanel moves the panel from where the finger left it to where it settles with a spring that leaves at the
// finger's speed (pixels a millisecond, positive to the right), and hands it back to the stylesheet at rest.
function settlePanel(
  { panel, backdrop }: { panel: HTMLElement; backdrop: HTMLElement | null },
  { open, offset, width, speed, onRest }: { open: boolean; offset: number; width: number; speed: number; onRest: () => void },
): SpringRun {
  return runSpring({
    from: offset,
    to: open ? 0 : -width,
    velocity: speed,
    // A fling fast enough to carry the panel past fully out stops it at the edge: there is nothing beyond it.
    onFrame: (position) => {
      const shown = Math.min(0, Math.max(-width, position));
      placePanel(panel, shown);
      placeBackdrop(backdrop, shown, width);
    },
    onRest: () => {
      handBack(panel, backdrop);
      onRest();
    },
    reducedMotion: document.documentElement.dataset.motion === "reduce",
  });
}

// useNavigationSwipe lets a phone open the navigation with a swipe to the right anywhere on the page and
// close it with a swipe to the left, the panel following the finger and the backdrop darkening with it. Let go,
// the panel carries on at the finger's speed to where it settles; a finger can catch it on the way. It does not
// wait at the screen's left edge, where Android and iOS take the swipe as Back.
export function useNavigationSwipe({ enabled, open, panelRef, backdropRef, onOpenChange }: {
  enabled: boolean;
  open: boolean;
  panelRef: RefObject<HTMLElement | null>;
  // backdropRef is the page's backdrop behind the open panel.
  backdropRef: RefObject<HTMLElement | null>;
  onOpenChange: (open: boolean) => void;
}) {
  const openRef = useRef(open);
  useEffect(() => {
    openRef.current = open;
  }, [open]);
  // Kept in a ref, so that a new function from a render does not take down a spring that is running.
  const onOpenChangeRef = useRef(onOpenChange);
  useEffect(() => {
    onOpenChangeRef.current = onOpenChange;
  }, [onOpenChange]);

  useEffect(() => {
    if (!enabled) return;
    let gesture: Gesture | null = null;
    // The spring finishing the last drag, while it runs, and what it moves.
    let settling: { run: SpringRun; panel: HTMLElement; backdrop: HTMLElement | null } | null = null;

    function start(event: TouchEvent) {
      // A second finger while one drags the panel is not a new swipe; the first finger's lifting settles it.
      if (gesture?.dragging === true) return;
      const panel = panelRef.current;
      const touch = event.touches[0];
      gesture = null;
      if (panel === null || touch === undefined || event.touches.length > 1) return;
      if (dialogOpen(panel) || ownsTheTouch(event.target)) return;
      gesture = {
        panel,
        backdrop: backdropRef.current,
        opening: !openRef.current,
        startX: touch.clientX,
        startY: touch.clientY,
        startTime: event.timeStamp,
        dragging: false,
        base: openRef.current ? 0 : -panel.getBoundingClientRect().width,
        offset: openRef.current ? 0 : -panel.getBoundingClientRect().width,
        width: panel.getBoundingClientRect().width,
        lastX: touch.clientX,
        lastTime: event.timeStamp,
        speed: 0,
      };
    }

    function move(event: TouchEvent) {
      const touch = event.touches[0];
      if (gesture === null || touch === undefined) return;
      const across = touch.clientX - gesture.startX;
      const down = touch.clientY - gesture.startY;
      if (!gesture.dragging) {
        if (Math.abs(across) < directionThreshold && Math.abs(down) < directionThreshold) return;
        const sideways = Math.abs(across) > Math.abs(down) * sidewaysRatio;
        const towardsSettling = gesture.opening ? across > 0 : across < 0;
        const pressedLong = event.timeStamp - gesture.startTime >= longPressMillis;
        if (!sideways || !towardsSettling || pressedLong) {
          gesture = null;
          return;
        }
        gesture.dragging = true;
        // A panel still on its way takes the finger where it is, rather than jumping back to where it started.
        if (settling !== null) {
          gesture.base = settling.run.stop();
          settling = null;
        }
      }
      // The page must not scroll under a finger that is moving the panel.
      event.preventDefault();
      gesture.offset = Math.min(0, Math.max(-gesture.width, gesture.base + across));
      placePanel(gesture.panel, gesture.offset);
      placeBackdrop(gesture.backdrop, gesture.offset, gesture.width);
      const elapsed = event.timeStamp - gesture.lastTime;
      if (elapsed > 0) gesture.speed = (touch.clientX - gesture.lastX) / elapsed;
      gesture.lastX = touch.clientX;
      gesture.lastTime = event.timeStamp;
    }

    function end() {
      const finished = gesture;
      gesture = null;
      if (finished === null || !finished.dragging) return;
      const travelled = Math.abs(finished.offset - finished.base);
      const flicked = finished.opening ? finished.speed > flickSpeed : finished.speed < -flickSpeed;
      const settles = travelled > finished.width * settleShare || flicked;
      const nextOpen = finished.opening ? settles : !settles;
      const run = settlePanel(
        { panel: finished.panel, backdrop: finished.backdrop },
        {
          open: nextOpen,
          offset: finished.offset,
          width: finished.width,
          speed: finished.speed,
          onRest: () => {
            settling = null;
          },
        },
      );
      settling = { run, panel: finished.panel, backdrop: finished.backdrop };
      if (nextOpen !== openRef.current) onOpenChangeRef.current(nextOpen);
    }

    document.addEventListener("touchstart", start, { passive: true });
    document.addEventListener("touchmove", move, { passive: false });
    document.addEventListener("touchend", end);
    document.addEventListener("touchcancel", end);
    return () => {
      document.removeEventListener("touchstart", start);
      document.removeEventListener("touchmove", move);
      document.removeEventListener("touchend", end);
      document.removeEventListener("touchcancel", end);
      // Swiping stops being offered mid-drag or mid-spring (the screen turned wider): the panel is handed back to the
      // stylesheet where its open state puts it.
      if (settling !== null) {
        settling.run.stop();
        handBack(settling.panel, settling.backdrop);
      }
      if (gesture?.dragging === true) handBack(gesture.panel, gesture.backdrop);
    };
  }, [enabled, panelRef, backdropRef]);
}
