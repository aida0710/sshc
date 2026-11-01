import type { IBuffer, IModes, Terminal } from "@xterm/xterm";
import { overlayClass } from "./selectionOverlay";
import type { Finger, TouchScrollTarget } from "./touchScroll";

type ScrolledTerminal = Pick<Terminal, "scrollLines"> & {
  readonly element: HTMLElement | undefined;
  readonly modes: Pick<IModes, "mouseTrackingMode">;
  readonly buffer: { readonly active: Pick<IBuffer, "type"> };
};

// The mouse tracking modes in which xterm.js reports the wheel to the program. X10 (DECSET 9) reports presses only.
const wheelReportingModes: ReadonlySet<IModes["mouseTrackingMode"]> = new Set(["vt200", "drag", "any"]);
// A program told of the wheel moves several lines for each step of it, as a mouse wheel turns a notch at a time:
// vim moves three by default ('mousescroll'). One step for each line the finger moves made the text run three times
// as fast as the finger, in jumps of three lines. A finger therefore turns the wheel one step for every three rows it
// moves. Without mouse reports xterm.js sends one arrow key per step, a line each, so there a step stays a row.
const rowsPerReportedWheelStep = 3;

// programScrollsItself is true while the program on the screen scrolls its own text: it asked for mouse reports, or it
// draws on the alternate screen, which has no scrollback (Claude Code does both; less and vim only the second).
function programScrollsItself(view: ScrolledTerminal): boolean {
  return wheelReportingModes.has(view.modes.mouseTrackingMode) || view.buffer.active.type === "alternate";
}

// terminalTouchScrollTarget sends a finger's scroll where xterm.js sends the touches it receives itself: to the
// program while it scrolls its own text, otherwise through the scrollback. On a touch screen the selection overlay
// lies over xterm.js and takes the touches, so for those sshc does what xterm.js would: it hands xterm.js wheel events
// (see rowsPerReportedWheelStep), and xterm.js turns each into a mouse report in the program's encoding, or into the
// up and down keys (in application cursor mode when the program set it). A touch that xterm.js receives itself (no overlay, as on a
// PC with both a touch screen and a mouse) already reaches the program through xterm.js, so for it sshc only moves
// the scrollback.
export function terminalTouchScrollTarget(view: ScrolledTerminal): TouchScrollTarget {
  // The rows the finger has moved toward the next reported wheel step, signed like the amount.
  let rowsTowardStep = 0;
  const wheelSteps = (amount: number): number => {
    if (!wheelReportingModes.has(view.modes.mouseTrackingMode)) return amount;
    // Turning back starts counting afresh, so the first rows the other way are not spent undoing the last ones.
    rowsTowardStep = Math.sign(rowsTowardStep) === -Math.sign(amount) ? amount : rowsTowardStep + amount;
    const steps = Math.trunc(rowsTowardStep / rowsPerReportedWheelStep);
    rowsTowardStep -= steps * rowsPerReportedWheelStep;
    return steps;
  };
  return {
    scrollLines(amount: number, finger: Finger) {
      const element = view.element;
      if (!startedOnOverlay(finger) || !programScrollsItself(view) || element === undefined) {
        view.scrollLines(amount);
        return;
      }
      const steps = wheelSteps(amount);
      for (let step = 0; step < Math.abs(steps); step += 1) {
        element.dispatchEvent(new WheelEvent("wheel", {
          deltaY: Math.sign(steps),
          deltaMode: WheelEvent.DOM_DELTA_LINE,
          clientX: finger.clientX,
          clientY: finger.clientY,
          cancelable: true,
        }));
      }
    },
  };
}

function startedOnOverlay(finger: Finger): boolean {
  return finger.touched instanceof Element && finger.touched.closest(`.${overlayClass}`) !== null;
}
