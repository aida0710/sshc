
export type Scroller = { rows: number; scrollLines(amount: number): void };
export function newTouchScroll(view: Scroller, cellHeight: () => number) {
  let last = 0;
  let carried = 0;

  return {
    start(y: number) {
      last = y;
      carried = 0;
    },
    move(y: number): number {
      const cell = cellHeight();
      if (cell <= 0) return 0;
      carried += (last - y) / cell;
      last = y;
      const lines = Math.trunc(carried);
      if (lines === 0) return 0;
      carried -= lines;
      view.scrollLines(lines);
      return lines;
    },
  };
}
