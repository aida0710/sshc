import { useRef, type TouchEvent as ReactTouchEvent } from "react";

const mobileBreakpoint = 768;
const edgeWidth = 28;
const minimumDistance = 64;
const maximumVerticalDistance = 48;

type TouchPoint = Readonly<{ clientX: number; clientY: number }>;
type TouchCollection = Readonly<{
  length: number;
  [index: number]: TouchPoint & Readonly<{ identifier: number }>;
}>;

export function shouldStartMobileNavigationSwipe(
  point: TouchPoint,
  viewportWidth: number,
  touchCount = 1,
): boolean {
  return viewportWidth < mobileBreakpoint && touchCount === 1 && point.clientX >= 0 && point.clientX <= edgeWidth;
}

export function shouldOpenMobileNavigation(start: TouchPoint, end: TouchPoint): boolean {
  const horizontal = end.clientX - start.clientX;
  const vertical = Math.abs(end.clientY - start.clientY);
  return horizontal >= minimumDistance && vertical <= maximumVerticalDistance && horizontal >= vertical * 1.5;
}

function viewportWidth(): number {
  return window.visualViewport?.width ?? window.innerWidth;
}

function touchByIdentifier(touches: TouchCollection, identifier: number) {
  for (let index = 0; index < touches.length; index += 1) {
    const touch = touches[index];
    if (touch?.identifier === identifier) return touch;
  }
  return undefined;
}

/** An invisible narrow-screen edge target which leaves the rest of the app untouched. */
export function MobileNavigationSwipeEdge({ onOpen }: { onOpen: () => void }) {
  const gesture = useRef<Readonly<{ identifier: number; start: TouchPoint }> | null>(null);
  const reset = () => {
    gesture.current = null;
  };
  const start = (event: ReactTouchEvent<HTMLDivElement>) => {
    const touch = event.touches[0];
    if (touch === undefined || !shouldStartMobileNavigationSwipe(touch, viewportWidth(), event.touches.length)) {
      reset();
      return;
    }
    gesture.current = {
      identifier: touch.identifier,
      start: { clientX: touch.clientX, clientY: touch.clientY },
    };
  };
  const move = (event: ReactTouchEvent<HTMLDivElement>) => {
    const current = gesture.current;
    if (current === null || event.touches.length !== 1) {
      reset();
      return;
    }
    const touch = touchByIdentifier(event.touches, current.identifier);
    if (touch === undefined) {
      reset();
      return;
    }
    const horizontal = Math.abs(touch.clientX - current.start.clientX);
    const vertical = Math.abs(touch.clientY - current.start.clientY);
    if (vertical > 24 && vertical > horizontal) {
      reset();
      return;
    }
    if (shouldOpenMobileNavigation(current.start, touch)) {
      event.preventDefault();
      reset();
      onOpen();
    }
  };
  return (
    <div
      aria-hidden="true"
      data-mobile-navigation-swipe="enabled"
      className="fixed inset-y-14 left-0 z-10 w-7 touch-pan-y md:hidden"
      onTouchStart={start}
      onTouchMove={move}
      onTouchEnd={reset}
      onTouchCancel={reset}
    />
  );
}
