import type { KeyboardEvent as ReactKeyboardEvent, PointerEvent as ReactPointerEvent } from "react";

export type SplitDirection = "horizontal" | "vertical";

export const minimumSplitRatio = 10;
export const maximumSplitRatio = 90;
const keyboardStep = 5;

export function clampSplitRatio(ratio: number): number {
  return Number.isFinite(ratio) ? Math.min(maximumSplitRatio, Math.max(minimumSplitRatio, Math.round(ratio))) : 50;
}

// SplitResizeHandle is the thin bar between the two halves of a split. It sits
// between them as a flex sibling and reports the first half's share of the
// parent as a percentage, so the parent can lay both halves out with flexBasis.
export function SplitResizeHandle({
  direction,
  ratio,
  label,
  onRatioChange,
}: {
  direction: SplitDirection;
  ratio: number;
  label: string;
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
      onRatioChange(clampSplitRatio(offset / extent * 100));
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
    onRatioChange(clampSplitRatio(ratio + (decrease ? -keyboardStep : keyboardStep)));
  }

  return (
    <div
      role="separator"
      tabIndex={0}
      aria-label={label}
      aria-orientation={row ? "vertical" : "horizontal"}
      aria-valuemin={minimumSplitRatio}
      aria-valuemax={maximumSplitRatio}
      aria-valuenow={ratio}
      onPointerDown={beginResize}
      onKeyDown={resizeWithKeyboard}
      className={`shrink-0 touch-none bg-line transition-colors hover:bg-accent focus:bg-accent focus:outline-none ${row ? "w-1 cursor-col-resize" : "h-1 cursor-row-resize"}`}
    />
  );
}
