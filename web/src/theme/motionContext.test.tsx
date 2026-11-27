import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MotionProvider, useReducedMotion } from "./motionContext";

// systemReducesMotion makes the OS answer the reduced-motion query with reduces.
function systemReducesMotion(reduces: boolean) {
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches: query === "(prefers-reduced-motion: reduce)" && reduces,
    media: query,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
  }));
}

function Probe() {
  return <span>{useReducedMotion() ? "reduced" : "animated"}</span>;
}

afterEach(() => {
  vi.restoreAllMocks();
  document.documentElement.removeAttribute("data-motion");
});

describe("MotionProvider", () => {
  it("leaves animation out while the OS asks to reduce motion", () => {
    systemReducesMotion(true);
    render(<MotionProvider><Probe /></MotionProvider>);

    expect(document.documentElement).toHaveAttribute("data-motion", "reduce");
    expect(screen.getByText("reduced")).toBeInTheDocument();
  });

  it("animates while the OS does not ask to reduce motion", () => {
    systemReducesMotion(false);
    render(<MotionProvider><Probe /></MotionProvider>);

    expect(document.documentElement).not.toHaveAttribute("data-motion");
    expect(screen.getByText("animated")).toBeInTheDocument();
  });
});
