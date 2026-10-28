import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { SplitResizeHandle } from "./SplitResizeHandle";

// The handle measures the split it divides: its parent, 760px tall here.
function renderHandle(onRatioChange: (ratio: number) => void) {
  const { container } = render(
    <div>
      <SplitResizeHandle direction="vertical" ratio={15} label="Resize" minimum={{ first: 80, second: 80 }} onRatioChange={onRatioChange} />
    </div>,
  );
  const split = container.firstElementChild as HTMLElement;
  split.getBoundingClientRect = () => new DOMRect(0, 0, 400, 760);
  return screen.getByRole("separator", { name: "Resize" });
}

describe("SplitResizeHandle", () => {
  it("stops the keyboard where the first half would get smaller than its minimum", () => {
    const onRatioChange = vi.fn();
    const handle = renderHandle(onRatioChange);

    fireEvent.keyDown(handle, { key: "ArrowUp" });

    // 15 - 5 = 10% would leave 76px of 760px; 80px needs 11%.
    expect(onRatioChange).toHaveBeenCalledExactlyOnceWith(11);
  });

  it("stops a drag at either half's minimum", () => {
    const onRatioChange = vi.fn();
    const handle = renderHandle(onRatioChange);
    handle.setPointerCapture = vi.fn();

    fireEvent.pointerDown(handle, { pointerId: 1 });
    fireEvent.pointerMove(window, { clientY: 5 });
    fireEvent.pointerMove(window, { clientY: 755 });
    fireEvent.pointerUp(window);

    expect(onRatioChange.mock.calls).toEqual([[11], [89]]);
  });
});
