import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";

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

afterEach(() => cleanup());
