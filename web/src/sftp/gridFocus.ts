// How the arrow keys move in a grid, a pure question of positions. Left and right walk the display order. Up and down
// keep the column, and down from a row above a shorter last row lands on that row's last cell. At an edge the focus
// stays where it is.
export type GridArrowKey = "ArrowLeft" | "ArrowRight" | "ArrowUp" | "ArrowDown";

export function gridArrowTarget({ key, from, count, columns }: { key: GridArrowKey; from: number; count: number; columns: number }): number {
  const last = count - 1;
  switch (key) {
    case "ArrowLeft":
      return Math.max(0, from - 1);
    case "ArrowRight":
      return Math.min(last, from + 1);
    case "ArrowUp":
      return from - columns < 0 ? from : from - columns;
    case "ArrowDown": {
      const hasRowBelow = Math.floor(from / columns) < Math.floor(last / columns);
      return hasRowBelow ? Math.min(last, from + columns) : from;
    }
  }
}

export function isGridArrowKey(key: string): key is GridArrowKey {
  return key === "ArrowLeft" || key === "ArrowRight" || key === "ArrowUp" || key === "ArrowDown";
}

// gridColumnCount reads how many columns the browser has laid out, which follows the pane's width.
export function gridColumnCount(grid: Element): number {
  return Math.max(1, getComputedStyle(grid).gridTemplateColumns.split(" ").length);
}
