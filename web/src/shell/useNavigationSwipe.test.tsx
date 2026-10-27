import { act, render, screen } from "@testing-library/react";
import { useRef, useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useNavigationSwipe } from "./useNavigationSwipe";

const panelWidth = 300;

function Harness({ initiallyOpen = false, onOpenChange }: { initiallyOpen?: boolean; onOpenChange: (open: boolean) => void }) {
  const [open, setOpen] = useState(initiallyOpen);
  const panelRef = useRef<HTMLElement>(null);
  useNavigationSwipe({
    enabled: true,
    open,
    panelRef,
    onOpenChange: (next) => {
      setOpen(next);
      onOpenChange(next);
    },
  });
  return (
    <div>
      <nav ref={panelRef} aria-label="navigation" />
      <p>page</p>
      <div data-terminal-host="">terminal</div>
      <span>{open ? "open" : "closed"}</span>
    </div>
  );
}

// touch sends one touch event the way a browser does: touches lists the fingers still down, and the event's
// time is what the hook measures a flick with.
function touch(target: Element, type: "touchstart" | "touchmove" | "touchend", x: number, y: number, at: number) {
  const event = new Event(type, { bubbles: true, cancelable: true });
  Object.defineProperty(event, "touches", { value: type === "touchend" ? [] : [{ clientX: x, clientY: y }] });
  Object.defineProperty(event, "timeStamp", { value: at });
  act(() => {
    target.dispatchEvent(event);
  });
  return event;
}

// swipe moves a finger from one point to another in steps, slowly enough not to count as a flick.
function swipe(target: Element, from: [number, number], to: [number, number]) {
  const steps = 10;
  touch(target, "touchstart", from[0], from[1], 0);
  for (let step = 1; step <= steps; step += 1) {
    const x = from[0] + ((to[0] - from[0]) * step) / steps;
    const y = from[1] + ((to[1] - from[1]) * step) / steps;
    touch(target, "touchmove", x, y, step * 100);
  }
  touch(target, "touchend", to[0], to[1], steps * 100 + 10);
}

function renderHarness(initiallyOpen = false) {
  const onOpenChange = vi.fn();
  render(<Harness initiallyOpen={initiallyOpen} onOpenChange={onOpenChange} />);
  const panel = screen.getByRole("navigation", { name: "navigation" });
  vi.spyOn(panel, "getBoundingClientRect").mockReturnValue({ width: panelWidth } as DOMRect);
  return { onOpenChange, panel };
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("useNavigationSwipe", () => {
  it("opens the navigation when a finger drags it out past a third of its width", () => {
    const { onOpenChange, panel } = renderHarness();

    swipe(screen.getByText("page"), [150, 400], [270, 410]);

    expect(onOpenChange).toHaveBeenCalledWith(true);
    expect(screen.getByText("open")).toBeInTheDocument();
    expect(panel.style.transform).toBe("");
  });

  it("lets the panel go back when the drag is short and slow", () => {
    const { onOpenChange } = renderHarness();

    swipe(screen.getByText("page"), [150, 400], [200, 400]);

    expect(onOpenChange).not.toHaveBeenCalled();
    expect(screen.getByText("closed")).toBeInTheDocument();
  });

  it("follows the finger while it drags", () => {
    const { panel } = renderHarness();
    const page = screen.getByText("page");

    touch(page, "touchstart", 100, 400, 0);
    const moving = touch(page, "touchmove", 220, 400, 100);

    expect(panel.style.transform).toBe(`translateX(${120 - panelWidth}px)`);
    expect(moving.defaultPrevented).toBe(true);
  });

  it("leaves a scroll up or down to the page", () => {
    const { onOpenChange, panel } = renderHarness();

    swipe(screen.getByText("page"), [150, 600], [190, 200]);

    expect(onOpenChange).not.toHaveBeenCalled();
    expect(panel.style.transform).toBe("");
  });

  it("does not take a swipe that starts on the terminal", () => {
    const { onOpenChange } = renderHarness();

    swipe(screen.getByText("terminal"), [100, 400], [300, 400]);

    expect(onOpenChange).not.toHaveBeenCalled();
  });

  it("hands the panel back to the stylesheet when the finger left it where it settles", () => {
    const { onOpenChange, panel } = renderHarness();
    const page = screen.getByText("page");

    // Dragged all the way out: nothing is left to animate, so no transition would end.
    swipe(page, [10, 400], [10 + panelWidth + 40, 400]);

    expect(onOpenChange).toHaveBeenCalledWith(true);
    expect(panel.style.transform).toBe("");
    expect(panel.dataset.navigationDragging).toBeUndefined();
  });

  it("does not let a second finger leave the panel half out", () => {
    const { onOpenChange, panel } = renderHarness();
    const page = screen.getByText("page");

    touch(page, "touchstart", 100, 400, 0);
    touch(page, "touchmove", 200, 400, 100);
    touch(page, "touchstart", 150, 500, 150);
    touch(page, "touchmove", 260, 400, 200);
    touch(page, "touchend", 260, 400, 260);

    expect(onOpenChange).toHaveBeenCalledWith(true);
    expect(panel.style.transform).toBe("");
  });

  it("closes the open navigation with a swipe to the left", () => {
    const { onOpenChange } = renderHarness(true);

    swipe(screen.getByText("page"), [280, 400], [100, 400]);

    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(screen.getByText("closed")).toBeInTheDocument();
  });
});
