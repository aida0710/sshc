import { describe, expect, it } from "vitest";
import { createAquariumArt } from "./aquariumArt";
import { asciiColorNames, beginFrame, type AsciiGrid } from "./asciiGrid";
import type { ArtInput } from "./artInput";

function gridOf(columns: number, rows: number): AsciiGrid {
  const frame = () => ({ characters: new Uint16Array(columns * rows), colors: new Uint8Array(columns * rows), levels: new Uint8Array(columns * rows) });
  return {
    context: {} as CanvasRenderingContext2D, width: columns * 8, height: rows * 17, cellWidth: 8, cellHeight: 17, baseline: 13, columns, rows, intensity: 1,
    colors: Object.fromEntries(asciiColorNames.map((name) => [name, name])) as AsciiGrid["colors"],
    shown: frame(), next: frame(),
  };
}

const charactersIn = (grid: AsciiGrid, color: string) => {
  const index = asciiColorNames.indexOf(color as (typeof asciiColorNames)[number]);
  return [...grid.next.characters].filter((character, cell) => character !== 0 && grid.next.colors[cell] === index).map((character) => String.fromCharCode(character));
};

describe("the aquarium", () => {
  it("draws the same tank for the same grid and the same fish", () => {
    const fish = [{ label: "web-01", weight: 9 }, { label: "db", weight: 2 }];
    const first = gridOf(90, 30);
    const second = gridOf(90, 30);
    const drawFirst = createAquariumArt(first, { fish, labels: true, swimDepth: 1 });
    const drawSecond = createAquariumArt(second, { fish, labels: true, swimDepth: 1 });
    beginFrame(first);
    drawFirst(6, { pointer: null, taps: [] });
    beginFrame(second);
    drawSecond(6, { pointer: null, taps: [] });
    expect([...first.next.characters]).toEqual([...second.next.characters]);
  });

  it("writes a fish's name under it", () => {
    const grid = gridOf(90, 30);
    createAquariumArt(grid, { fish: [{ label: "web-01", weight: 9 }], labels: true, swimDepth: 1 })(6, { pointer: null, taps: [] });
    expect(charactersIn(grid, "ink").join("")).toContain("web-01");
  });

  it("drops food where it is tapped, and the fish eat it", () => {
    const grid = gridOf(60, 16);
    const draw = createAquariumArt(grid, { fish: [{ label: "web-01", weight: 9 }, { label: "db", weight: 1 }], labels: false, swimDepth: 1 });
    const input: ArtInput = { pointer: null, taps: [{ column: 30, row: 4 }] };
    beginFrame(grid);
    draw(6, input);
    expect(input.taps).toEqual([]);
    expect(charactersIn(grid, "yellow")).toContain("*");

    // The fish race to the food; a name shows while its fish eats, even with names off.
    let named = false;
    for (let frame = 1; frame < 400; frame++) {
      beginFrame(grid);
      draw(6 + frame / 10, input);
      named ||= /web-01|db/.test(charactersIn(grid, "ink").join(""));
    }
    expect(named).toBe(true);
  });
});
