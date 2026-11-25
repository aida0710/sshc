import { afterEach, describe, expect, it } from "vitest";
import { followArtInput, type ArtInput } from "./artInput";

// A canvas at (100, 50), 400 by 200 CSS pixels, cut into cells 10 wide and 20 tall.
function laidOutCanvas(): HTMLCanvasElement {
  const canvas = document.createElement("canvas");
  canvas.getBoundingClientRect = () => ({ left: 100, top: 50, width: 400, height: 200, right: 500, bottom: 250, x: 100, y: 50, toJSON: () => ({}) });
  document.body.append(canvas);
  return canvas;
}

const cells = { cellWidth: 10, cellHeight: 20, columns: 40, rows: 10 };

function pointer(type: string, at: { x: number; y: number }, pointerType = "mouse", target: EventTarget = document.body): void {
  const event = new MouseEvent(type, { clientX: at.x, clientY: at.y, bubbles: true });
  Object.defineProperty(event, "pointerType", { value: pointerType });
  target.dispatchEvent(event);
}

let stop = () => undefined as void;
afterEach(() => {
  stop();
  document.body.replaceChildren();
});

describe("followArtInput", () => {
  it("places the mouse on the cell under it, and forgets it outside the art", () => {
    const input: ArtInput = { pointer: null, taps: [] };
    stop = followArtInput(laidOutCanvas(), cells, input);

    pointer("pointermove", { x: 135, y: 95 });
    expect(input.pointer).toEqual({ column: 3, row: 2 });

    pointer("pointermove", { x: 20, y: 20 });
    expect(input.pointer).toBeNull();
  });

  it("does not take a finger moving for a mouse", () => {
    const input: ArtInput = { pointer: null, taps: [] };
    stop = followArtInput(laidOutCanvas(), cells, input);

    pointer("pointermove", { x: 135, y: 95 }, "touch");
    expect(input.pointer).toBeNull();
  });

  it("takes a tap on the art, but not a drag nor a press on a control", () => {
    const input: ArtInput = { pointer: null, taps: [] };
    stop = followArtInput(laidOutCanvas(), cells, input);
    const button = document.createElement("button");
    document.body.append(button);

    pointer("pointerdown", { x: 135, y: 95 }, "touch");
    pointer("pointerup", { x: 137, y: 96 }, "touch");
    pointer("pointerdown", { x: 135, y: 95 }, "touch");
    pointer("pointerup", { x: 135, y: 140 }, "touch");
    pointer("pointerdown", { x: 135, y: 95 }, "mouse", button);
    pointer("pointerup", { x: 135, y: 95 }, "mouse", button);

    expect(input.taps).toEqual([{ column: 3, row: 2 }]);
  });

  it("stops following when stopped", () => {
    const input: ArtInput = { pointer: null, taps: [] };
    followArtInput(laidOutCanvas(), cells, input)();

    pointer("pointermove", { x: 135, y: 95 });
    expect(input.pointer).toBeNull();
  });
});
