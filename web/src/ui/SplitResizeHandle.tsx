import type { CSSProperties, KeyboardEvent as ReactKeyboardEvent, PointerEvent as ReactPointerEvent } from "react";

export type SplitDirection = "horizontal" | "vertical";

const minimumSplitRatio = 10;
const maximumSplitRatio = 90;
const keyboardStep = 5;

export function clampSplitRatio(ratio: number): number {
  return Number.isFinite(ratio) ? Math.min(maximumSplitRatio, Math.max(minimumSplitRatio, Math.round(ratio))) : 50;
}

// SplitMinimum is how many px each half needs at least, along the split.
export type SplitMinimum = { first: number; second: number };

// fitSplitRatio is clampSplitRatio that also leaves each half its minimum in
// px of extentPx. When both minimums do not fit, they share what there is.
export function fitSplitRatio(ratio: number, extentPx: number, minimum: SplitMinimum): number {
  const rounded = clampSplitRatio(ratio);
  if (extentPx <= 0) return rounded;
  const lowest = Math.max(minimumSplitRatio, Math.ceil(minimum.first / extentPx * 100));
  const highest = Math.min(maximumSplitRatio, Math.floor(100 - minimum.second / extentPx * 100));
  if (lowest > highest) return clampSplitRatio(minimum.first / (minimum.first + minimum.second) * 100);
  return Math.min(highest, Math.max(lowest, rounded));
}

// SplitResizeHandle is the thin bar between the two halves of a split. It sits
// between them as a flex sibling and reports the first half's share of the
// parent as a percentage, so the parent can lay both halves out with flexBasis.
export function SplitResizeHandle({
  direction,
  ratio,
  label,
  onRatioChange,
  style,
  look = "line",
  minimum = { first: 0, second: 0 },
}: {
  direction: SplitDirection;
  ratio: number;
  label: string;
  style?: CSSProperties;
  // minimum keeps each half at least this many px, however far it is dragged.
  minimum?: SplitMinimum;
  // "line" draws the divider between two panes that touch; "gap" sits in the space between
  // two boards and shows only while the pointer is on it.
  look?: "line" | "gap";
  onRatioChange: (ratio: number) => void;
}) {
  const row = direction === "horizontal";

  function beginResize(event: ReactPointerEvent<HTMLDivElement>) {
    event.preventDefault();
    event.stopPropagation();
    const container = event.currentTarget.parentElement;
    if (container === null) return;
    event.currentTarget.setPointerCapture(event.pointerId);
    const move = (next: PointerEvent) => {
      const bounds = container.getBoundingClientRect();
      const extent = row ? bounds.width : bounds.height;
      if (extent <= 0) return;
      const offset = row ? next.clientX - bounds.left : next.clientY - bounds.top;
      onRatioChange(fitSplitRatio(offset / extent * 100, extent, minimum));
    };
    const stop = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", stop);
      window.removeEventListener("pointercancel", stop);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", stop, { once: true });
    window.addEventListener("pointercancel", stop, { once: true });
  }

  function resizeWithKeyboard(event: ReactKeyboardEvent<HTMLDivElement>) {
    const decrease = event.key === (row ? "ArrowLeft" : "ArrowUp");
    const increase = event.key === (row ? "ArrowRight" : "ArrowDown");
    if (!decrease && !increase) return;
    event.preventDefault();
    const bounds = event.currentTarget.parentElement?.getBoundingClientRect();
    const extent = bounds === undefined ? 0 : row ? bounds.width : bounds.height;
    onRatioChange(fitSplitRatio(ratio + (decrease ? -keyboardStep : keyboardStep), extent, minimum));
  }

  return (
    <div
      style={style}
      role="separator"
      tabIndex={0}
      aria-label={label}
      aria-orientation={row ? "vertical" : "horizontal"}
      aria-valuemin={minimumSplitRatio}
      aria-valuemax={maximumSplitRatio}
      aria-valuenow={ratio}
      onPointerDown={beginResize}
      onKeyDown={resizeWithKeyboard}
      className={`shrink-0 touch-none transition-colors hover:bg-accent focus:bg-accent focus:outline-none ${look === "line" ? "bg-line" : "bg-transparent"} ${row ? "w-1 cursor-col-resize" : "h-1 cursor-row-resize"}`}
    />
  );
}
