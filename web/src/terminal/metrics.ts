
export type MeasurableTerminal = {
  readonly element: HTMLElement | undefined;
  readonly rows: number;
};

export type CellMetrics = {
  readonly rect: DOMRect;
  readonly cellHeight: number;
  readonly font: {
    readonly family: string;
    readonly size: string;
    readonly weight: string;
    readonly letterSpacing: string;
  };
};
function surface(view: MeasurableTerminal): HTMLElement | null {
  return view.element?.querySelector<HTMLElement>(".xterm-screen") ?? null;
}
function perRow(height: number, rows: number): number {
  return rows <= 0 ? 0 : height / rows;
}
export function measureCells(view: MeasurableTerminal): CellMetrics | null {
  const screen = surface(view);
  const glyphs = view.element?.querySelector<HTMLElement>(".xterm-rows") ?? null;
  if (screen === null || glyphs === null || view.rows <= 0) return null;
  const rect = screen.getBoundingClientRect();
  if (rect.width <= 0 || rect.height <= 0) return null;
  const style = getComputedStyle(glyphs);
  return {
    rect,
    cellHeight: perRow(rect.height, view.rows),
    font: {
      family: style.fontFamily,
      size: style.fontSize,
      weight: style.fontWeight,
      letterSpacing: style.letterSpacing,
    },
  };
}
export function cellHeight(view: MeasurableTerminal, whileBuilding: HTMLElement): number {
  return perRow((surface(view) ?? whileBuilding).getBoundingClientRect().height, view.rows);
}
