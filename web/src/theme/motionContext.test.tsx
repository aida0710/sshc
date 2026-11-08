import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { fakeSettingsServer } from "../testing/settingsServer";
import { MotionProvider, reduceMotionChoiceOf, useReducedMotion, withReduceMotion } from "./motionContext";

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
  it("follows the OS while the preset leaves the choice out", async () => {
    systemReducesMotion(true);
    fakeSettingsServer();
    render(<MotionProvider><Probe /></MotionProvider>);

    await waitFor(() => expect(document.documentElement).toHaveAttribute("data-motion", "reduce"));
    expect(screen.getByText("reduced")).toBeInTheDocument();
  });

  it("reduces motion when the preset says on, even though the OS animates", async () => {
    systemReducesMotion(false);
    fakeSettingsServer({ presets: [{ id: "default", appearance: { reduceMotion: true } }] });
    render(<MotionProvider><Probe /></MotionProvider>);

    await waitFor(() => expect(document.documentElement).toHaveAttribute("data-motion", "reduce"));
    expect(screen.getByText("reduced")).toBeInTheDocument();
  });

  it("keeps animation when the preset says off, even though the OS reduces motion", async () => {
    systemReducesMotion(true);
    fakeSettingsServer({ presets: [{ id: "default", appearance: { reduceMotion: false } }] });
    render(<MotionProvider><Probe /></MotionProvider>);

    await waitFor(() => expect(screen.getByText("animated")).toBeInTheDocument());
    expect(document.documentElement).not.toHaveAttribute("data-motion");
  });
});

describe("reduce motion choice", () => {
  it("keeps following the OS as no value, so the preset looks like one saved before the setting existed", () => {
    const chosen = withReduceMotion({ id: "default", appearance: { accent: "sky", reduceMotion: true } }, "system");

    expect(chosen.appearance).toEqual({ accent: "sky" });
    expect(reduceMotionChoiceOf(chosen)).toBe("system");
  });

  it("reads back the choice it keeps", () => {
    expect(reduceMotionChoiceOf(withReduceMotion({ id: "default" }, "on"))).toBe("on");
    expect(reduceMotionChoiceOf(withReduceMotion({ id: "default" }, "off"))).toBe("off");
  });
});
