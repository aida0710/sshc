
export type MeasurableTerminal = {
  readonly element: HTMLElement | undefined;
  readonly rows: number;
  readonly options?: {
    fontFamily?: string;
    fontSize?: number;
    fontWeight?: string | number;
    letterSpacing?: number;
  };
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

export function observeTerminalSize(
  view: MeasurableTerminal,
  host: HTMLElement,
  onResize: () => void,
): () => void {
  const screen = surface(view);
  const sizeKey = () => {
    const hostBox = host.getBoundingClientRect();
    const screenBox = screen?.getBoundingClientRect();
    return [hostBox.width, hostBox.height, screenBox?.width, screenBox?.height].join(":");
  };
  let fittedSize = sizeKey();
  let frame: number | null = null;
  const observer = new ResizeObserver(() => {
    if (frame !== null || sizeKey() === fittedSize) return;
    // Fitting changes the screen itself. Wait for the next frame so observing
    // its size cannot cause a ResizeObserver delivery loop.
    frame = window.requestAnimationFrame(() => {
      frame = null;
      if (sizeKey() === fittedSize) return;
      onResize();
      // Ignore the size change caused by this fit. Fractional pixel rounding
      // can otherwise make FitAddon alternate between two row counts forever.
      fittedSize = sizeKey();
    });
  });
  observer.observe(host);
  // Renderer fallback, font loading and DPI changes can change the cell size
  // without resizing the host. All rows must still fit in the visible pane.
  if (screen !== null) observer.observe(screen);
  return () => {
    observer.disconnect();
    if (frame !== null) window.cancelAnimationFrame(frame);
  };
}

function perRow(height: number, rows: number): number {
  return rows <= 0 ? 0 : height / rows;
}
export function measureCells(view: MeasurableTerminal): CellMetrics | null {
  const screen = surface(view);
  const glyphs = view.element?.querySelector<HTMLElement>(".xterm-rows") ?? null;
  if (screen === null || view.rows <= 0) return null;
  const rect = screen.getBoundingClientRect();
  if (rect.width <= 0 || rect.height <= 0) return null;
  const style = getComputedStyle(glyphs ?? screen);
  // WebGL replaces the DOM rows after the first frame. The public terminal
  // options remain available across renderer changes and font adjustments.
  const font = glyphs === null ? view.options : undefined;
  return {
    rect,
    cellHeight: perRow(rect.height, view.rows),
    font: {
      family: font?.fontFamily ?? style.fontFamily,
      size: font?.fontSize === undefined ? style.fontSize : `${font.fontSize}px`,
      weight: font?.fontWeight === undefined ? style.fontWeight : String(font.fontWeight),
      letterSpacing: font?.letterSpacing === undefined ? style.letterSpacing : `${font.letterSpacing}px`,
    },
  };
}

export function syncTerminalInputPosition(view: MeasurableTerminal & {
  readonly cols: number;
  readonly textarea: HTMLTextAreaElement | undefined;
  readonly buffer: { readonly active: { readonly cursorX: number; readonly cursorY: number } };
}): void {
  const textarea = view.textarea;
  const cells = measureCells(view);
  if (textarea === undefined || cells === null || view.cols <= 0) return;
  // xterm updates its IME textarea on cursor movement, but resizing can move
  // the buffer cursor without that event. Android blurs an input left outside
  // the shrunken viewport and immediately closes the keyboard again.
  const cursor = view.buffer.active;
  textarea.style.top = `${Math.max(0, Math.min(view.rows - 1, cursor.cursorY)) * cells.cellHeight}px`;
  textarea.style.left = `${Math.max(0, Math.min(view.cols - 1, cursor.cursorX)) * cells.rect.width / view.cols}px`;
}
export function cellHeight(view: MeasurableTerminal, whileBuilding: HTMLElement): number {
  return perRow((surface(view) ?? whileBuilding).getBoundingClientRect().height, view.rows);
}
