import type { IModes } from "@xterm/xterm";
import { describe, expect, it } from "vitest";
import { overlayClass } from "./selectionOverlay";
import { terminalTouchScrollTarget } from "./touchScrollTarget";

function terminal({ mouse = "none", screen = "normal" }: { mouse?: IModes["mouseTrackingMode"]; screen?: "normal" | "alternate" } = {}) {
  const element = document.createElement("div");
  const wheels: WheelEvent[] = [];
  element.addEventListener("wheel", (event) => wheels.push(event));
  const scrolled: number[] = [];
  const view = {
    element,
    modes: { mouseTrackingMode: mouse },
    buffer: { active: { type: screen } },
    scrollLines: (amount: number) => scrolled.push(amount),
  };
  return { target: terminalTouchScrollTarget(view), wheels, scrolled };
}

const overlay = document.createElement("pre");
overlay.className = overlayClass;
const onOverlay = { clientX: 120, clientY: 300, touched: overlay };
const onXterm = { clientX: 120, clientY: 300, touched: document.createElement("canvas") };

describe("terminalTouchScrollTarget", () => {
  it("moves the scrollback while the shell is on the normal screen", () => {
    const { target, wheels, scrolled } = terminal();
    target.scrollLines(3, onOverlay);
    expect(scrolled).toEqual([3]);
    expect(wheels).toEqual([]);
  });

  it("turns each line into a wheel turn at the finger for a program that asks for mouse reports", () => {
    const { target, wheels, scrolled } = terminal({ mouse: "any", screen: "alternate" });
    target.scrollLines(2, onOverlay);
    target.scrollLines(-1, onOverlay);
    expect(scrolled).toEqual([]);
    expect(wheels.map((wheel) => [wheel.deltaY, wheel.deltaMode, wheel.clientX, wheel.clientY])).toEqual([
      [1, WheelEvent.DOM_DELTA_LINE, 120, 300],
      [1, WheelEvent.DOM_DELTA_LINE, 120, 300],
      [-1, WheelEvent.DOM_DELTA_LINE, 120, 300],
    ]);
  });

  it("turns the wheel for a program on the alternate screen that does not ask for mouse reports", () => {
    const { target, wheels, scrolled } = terminal({ screen: "alternate" });
    target.scrollLines(-2, onOverlay);
    expect(scrolled).toEqual([]);
    expect(wheels.map((wheel) => wheel.deltaY)).toEqual([-1, -1]);
  });

  it("turns the wheel for a program on the normal screen that asks for wheel reports, but not for X10 presses only", () => {
    const reporting = terminal({ mouse: "vt200" });
    reporting.target.scrollLines(1, onOverlay);
    expect(reporting.wheels).toHaveLength(1);

    const pressesOnly = terminal({ mouse: "x10" });
    pressesOnly.target.scrollLines(1, onOverlay);
    expect(pressesOnly.wheels).toEqual([]);
    expect(pressesOnly.scrolled).toEqual([1]);
  });

  it("leaves a touch that xterm.js receives itself to xterm.js, so the program is not told twice", () => {
    const { target, wheels } = terminal({ mouse: "any", screen: "alternate" });
    target.scrollLines(2, onXterm);
    expect(wheels).toEqual([]);
  });
});
