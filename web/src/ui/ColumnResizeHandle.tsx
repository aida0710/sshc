import { useEffect, useRef, useState, type KeyboardEvent as ReactKeyboardEvent, type PointerEvent as ReactPointerEvent } from "react";

// ColumnResizeHandle is the thin vertical grip on the right edge of a column.
// The parent positions it (usually `relative`) and owns the width; the handle
// only turns pointer drags and arrow keys into clamped width values.
export function ColumnResizeHandle({
  label,
  width,
  minimum,
  maximum,
  onWidthChange,
  className = "hidden md:flex",
}: {
  label: string;
  width: number;
  minimum: number;
  maximum: number;
  onWidthChange: (width: number) => void;
  className?: string;
}) {
  const drag = useRef<{ pointerId: number; startX: number; startWidth: number } | null>(null);
  const queuedWidth = useRef(width);
  const animationFrame = useRef<number | null>(null);
  const previousUserSelect = useRef("");
  const clamp = (value: number) => Number.isFinite(value) ? Math.min(maximum, Math.max(minimum, Math.round(value))) : width;

  useEffect(() => {
    queuedWidth.current = width;
  }, [width]);

  useEffect(() => () => {
    if (animationFrame.current !== null) window.cancelAnimationFrame(animationFrame.current);
    if (drag.current !== null) document.body.style.userSelect = previousUserSelect.current;
  }, []);

  function publish(nextWidth: number) {
    queuedWidth.current = clamp(nextWidth);
    if (animationFrame.current !== null) return;
    animationFrame.current = window.requestAnimationFrame(() => {
      animationFrame.current = null;
      onWidthChange(queuedWidth.current);
    });
  }

  function finish(pointerId: number) {
    if (drag.current?.pointerId !== pointerId) return;
    drag.current = null;
    document.body.style.userSelect = previousUserSelect.current;
    if (animationFrame.current !== null) {
      window.cancelAnimationFrame(animationFrame.current);
      animationFrame.current = null;
      onWidthChange(queuedWidth.current);
    }
  }

  function start(event: ReactPointerEvent<HTMLDivElement>) {
    if (event.button !== 0 || drag.current !== null) return;
    event.preventDefault();
    drag.current = { pointerId: event.pointerId, startX: event.clientX, startWidth: width };
    queuedWidth.current = width;
    previousUserSelect.current = document.body.style.userSelect;
    document.body.style.userSelect = "none";
    event.currentTarget.setPointerCapture(event.pointerId);
  }

  function move(event: ReactPointerEvent<HTMLDivElement>) {
    const active = drag.current;
    if (active === null || active.pointerId !== event.pointerId) return;
    publish(active.startWidth + event.clientX - active.startX);
  }

  function useKeyboard(event: ReactKeyboardEvent<HTMLDivElement>) {
    let nextWidth: number | null = null;
    const step = event.shiftKey ? 32 : 8;
    if (event.key === "ArrowLeft") nextWidth = width - step;
    if (event.key === "ArrowRight") nextWidth = width + step;
    if (event.key === "Home") nextWidth = minimum;
    if (event.key === "End") nextWidth = maximum;
    if (nextWidth === null) return;
    event.preventDefault();
    onWidthChange(clamp(nextWidth));
  }

  return (
    <div
      role="separator"
      aria-label={label}
      aria-orientation="vertical"
      aria-valuemin={minimum}
      aria-valuemax={maximum}
      aria-valuenow={width}
      tabIndex={0}
      onPointerDown={start}
      onPointerMove={move}
      onPointerUp={(event) => finish(event.pointerId)}
      onPointerCancel={(event) => finish(event.pointerId)}
      onLostPointerCapture={(event) => finish(event.pointerId)}
      onKeyDown={useKeyboard}
      className={`group absolute inset-y-0 right-0 z-10 w-2 cursor-col-resize touch-none items-center justify-center outline-none ${className}`}
    >
      <span
        aria-hidden="true"
        className="h-full w-px bg-transparent transition-colors group-hover:bg-accent group-focus-visible:w-0.5 group-focus-visible:bg-accent"
      />
    </div>
  );
}

// useStoredColumnWidth keeps one column width per browser. Layout preferences
// are conveniences, so a blocked localStorage silently falls back to the default.
export function useStoredColumnWidth(
  key: string,
  fallback: number,
  minimum: number,
  maximum: number,
): [number, (width: number) => void] {
  const clamp = (value: number) => Number.isFinite(value) ? Math.min(maximum, Math.max(minimum, Math.round(value))) : fallback;
  const [width, setWidth] = useState(() => {
    try {
      const stored = window.localStorage.getItem(key);
      if (stored !== null && stored.trim() !== "") return clamp(Number(stored));
    } catch {
      // Private browsing policies may disable localStorage.
    }
    return fallback;
  });
  const change = (next: number) => {
    const clamped = clamp(next);
    setWidth(clamped);
    try {
      window.localStorage.setItem(key, String(clamped));
    } catch {
      // Losing the preference is acceptable; the width still applies now.
    }
  };
  return [width, change];
}
