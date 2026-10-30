import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";
import { resetSettingsForTests } from "./src/settings/settingsStore";

// jsdom leaves canvas rendering unimplemented. xterm 6.1's DOM renderer
// measures glyph widths on a 2D canvas and will not start without one, so the
// 2D context here measures every character as one 8px cell and ignores
// drawing. WebGL stays unsupported, which is the browser fallback under test.
const testGlyphWidthPx = 8;

function measuringContext(canvas: HTMLCanvasElement): CanvasRenderingContext2D {
  const state: Record<PropertyKey, unknown> = {
    canvas,
    font: "10px monospace",
    measureText: (text: string) => ({ width: [...text].length * testGlyphWidthPx }),
  };
  return new Proxy(state, {
    get: (target, property) => (property in target ? target[property] : () => undefined),
    set: (target, property, value) => {
      target[property] = value;
      return true;
    },
  }) as unknown as CanvasRenderingContext2D;
}

Object.defineProperty(HTMLCanvasElement.prototype, "getContext", {
  configurable: true,
  value(this: HTMLCanvasElement, kind: string) {
    return kind === "2d" ? measuringContext(this) : null;
  },
});

// Unmounting runs the effects React still had pending, and one may store a choice (an update result
// shown, a folded notice). Storage is cleared after that, so no test starts with what the last one stored.
afterEach(() => {
  cleanup();
  localStorage.clear();
  sessionStorage.clear();
  resetSettingsForTests();
});
