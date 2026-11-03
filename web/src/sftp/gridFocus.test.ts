import { describe, expect, it } from "vitest";
import { gridArrowTarget, isGridArrowKey } from "./gridFocus";

// Seven cells in three columns:
// 0 1 2
// 3 4 5
// 6
const inGrid = { count: 7, columns: 3 };

describe("gridArrowTarget", () => {
  it("walks the display order with left and right, across the ends of rows", () => {
    expect(gridArrowTarget({ ...inGrid, key: "ArrowRight", from: 2 })).toBe(3);
    expect(gridArrowTarget({ ...inGrid, key: "ArrowLeft", from: 3 })).toBe(2);
  });

  it("keeps the column with up and down", () => {
    expect(gridArrowTarget({ ...inGrid, key: "ArrowDown", from: 1 })).toBe(4);
    expect(gridArrowTarget({ ...inGrid, key: "ArrowUp", from: 4 })).toBe(1);
  });

  it("goes down onto the last cell of a shorter last row", () => {
    expect(gridArrowTarget({ ...inGrid, key: "ArrowDown", from: 5 })).toBe(6);
  });

  it("stays put at the edges", () => {
    expect(gridArrowTarget({ ...inGrid, key: "ArrowUp", from: 2 })).toBe(2);
    expect(gridArrowTarget({ ...inGrid, key: "ArrowDown", from: 6 })).toBe(6);
    expect(gridArrowTarget({ ...inGrid, key: "ArrowLeft", from: 0 })).toBe(0);
    expect(gridArrowTarget({ ...inGrid, key: "ArrowRight", from: 6 })).toBe(6);
  });
});

it("knows the arrow keys from the rest", () => {
  expect(isGridArrowKey("ArrowLeft")).toBe(true);
  expect(isGridArrowKey("Enter")).toBe(false);
});
