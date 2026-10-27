import { useEffect, useRef, type RefObject } from "react";

// How far a finger moves before the swipe is read as sideways or up and down. Below it a tap stays a tap.
const directionThreshold = 10;
// A swipe counts as sideways when it moves this many times further across than up or down, so scrolling a
// page with a slightly slanted finger does not pull the navigation out.
const sidewaysRatio = 1.2;
// Letting go past this share of the panel's width keeps it where the finger took it.
const settleShare = 0.35;
// A flick this fast (px per ms) settles it the same way, however short it was.
const flickSpeed = 0.35;

// Where a swipe must not start: the terminal reads touches itself (scrolling, selecting), text fields
// select by dragging, and anything that scrolls sideways needs the sideways swipe for itself.
const ownsTouchSelector = "[data-terminal-host], input, textarea, select, [contenteditable='true']";

type Gesture = {
  // The panel the finger moves, taken when the finger goes down.
  panel: HTMLElement;
  opening: boolean;
  // Where the panel is while it follows the finger, from 0 (open) to -width (closed).
  offset: number;
  startX: number;
  startY: number;
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

function ownsTheTouch(target: EventTarget | null): boolean {
  if (!(target instanceof Element)) return false;
  if (target.closest(ownsTouchSelector) !== null) return true;
  for (let element: Element | null = target; element !== null && element !== document.body; element = element.parentElement) {
    if (scrollsSideways(element)) return true;
  }
  return false;
}

// A dialog over the page takes every touch; swiping behind it must not move the navigation.
function dialogOpen(panel: HTMLElement): boolean {
  return Array.from(document.querySelectorAll("[aria-modal='true']")).some((dialog) => !panel.contains(dialog));
}

// While a finger drags the panel, its position is written inline: transform for where it is, and translate
// cleared so the closed state's -translate-x-full does not add to it. Once let go, the inline position moves
// to where the panel settles, with the panel's own transition, and is handed back to the stylesheet when that
// transition ends, by which time the open state says the same.
function placePanel(panel: HTMLElement, offset: number) {
  panel.dataset.navigationDragging = "";
  panel.style.transition = "none";
  panel.style.translate = "none";
  panel.style.transform = `translateX(${offset}px)`;
}

// settlePanel moves the panel from offset to where it settles. A panel already there has nothing to
// animate, and no transition would end to hand it back, so it is handed back at once.
function settlePanel(panel: HTMLElement, { open, offset, width }: { open: boolean; offset: number; width: number }) {
  delete panel.dataset.navigationDragging;
  panel.style.transition = "";
  panel.style.transform = open ? "translateX(0)" : "translateX(-100%)";
  const release = () => {
    panel.removeEventListener("transitionend", ended);
    panel.removeEventListener("transitioncancel", ended);
    panel.style.transform = "";
    panel.style.translate = "";
  };
  // Only the panel's own transition counts: its children's end too and bubble up to it.
  const ended = (event: TransitionEvent) => {
    if (event.target === panel) release();
  };
  const settled = Math.abs(offset - (open ? 0 : -width)) < 1;
  const transitions = getComputedStyle(panel).transitionDuration.split(",").some((duration) => Number.parseFloat(duration) > 0);
  if (settled || !transitions) {
    release();
    return;
  }
  panel.addEventListener("transitionend", ended);
  panel.addEventListener("transitioncancel", ended);
}

// useNavigationSwipe lets a phone open the navigation with a swipe to the right anywhere on the page and
// close it with a swipe to the left, the panel following the finger. It does not wait at the screen's left
// edge, where Android and iOS take the swipe as Back.
export function useNavigationSwipe({ enabled, open, panelRef, onOpenChange }: {
  enabled: boolean;
  open: boolean;
  panelRef: RefObject<HTMLElement | null>;
  onOpenChange: (open: boolean) => void;
}) {
  const openRef = useRef(open);
  useEffect(() => {
    openRef.current = open;
  }, [open]);

  useEffect(() => {
    if (!enabled) return;
    let gesture: Gesture | null = null;

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
        opening: !openRef.current,
        startX: touch.clientX,
        startY: touch.clientY,
        dragging: false,
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
        if (!sideways || !towardsSettling) {
          gesture = null;
          return;
        }
        gesture.dragging = true;
      }
      // The page must not scroll under a finger that is moving the panel.
      event.preventDefault();
      const offset = gesture.opening ? across - gesture.width : across;
      gesture.offset = Math.min(0, Math.max(-gesture.width, offset));
      placePanel(gesture.panel, gesture.offset);
      const elapsed = event.timeStamp - gesture.lastTime;
      if (elapsed > 0) gesture.speed = (touch.clientX - gesture.lastX) / elapsed;
      gesture.lastX = touch.clientX;
      gesture.lastTime = event.timeStamp;
    }

    function end() {
      const finished = gesture;
      gesture = null;
      if (finished === null || !finished.dragging) return;
      const travelled = Math.abs(finished.lastX - finished.startX);
      const flicked = finished.opening ? finished.speed > flickSpeed : finished.speed < -flickSpeed;
      const settles = travelled > finished.width * settleShare || flicked;
      const nextOpen = finished.opening ? settles : !settles;
      settlePanel(finished.panel, { open: nextOpen, offset: finished.offset, width: finished.width });
      if (nextOpen !== openRef.current) onOpenChange(nextOpen);
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
      // Swiping stops being offered mid-drag (the screen turned wider): the panel goes back where it was.
      if (gesture?.dragging === true) {
        settlePanel(gesture.panel, { open: openRef.current, offset: gesture.offset, width: gesture.width });
      }
    };
  }, [enabled, panelRef, onOpenChange]);
}
