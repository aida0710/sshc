
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

// terminalScreen is the element the rows are drawn in, for what is laid over
// them, such as the command marks (commandGutter.ts).
export function terminalScreen(view: MeasurableTerminal): HTMLElement | null {
  return surface(view);
}

// The part of xterm's render dimensions that a font or DPI change alters.
type CellDimensions = { readonly css: { readonly cell: { readonly width: number; readonly height: number } } };

export type ObservableTerminal = MeasurableTerminal & {
  onDimensionsChange(listener: (dimensions: CellDimensions) => void): { dispose(): void };
};

export function observeTerminalSize(
  view: ObservableTerminal,
  host: HTMLElement,
  onResize: () => void,
): () => void {
  const screen = surface(view);
  const sizeKey = () => {
    const hostBox = host.getBoundingClientRect();
    const screenBox = screen?.getBoundingClientRect();
    return [hostBox.width, hostBox.height, screenBox?.width, screenBox?.height].join(":");
  };
  // Only a size fitted here counts as fitted. Recording the current size without
  // fitting would ignore a layout that changed after the caller last fitted.
  onResize();
  let fittedSize = sizeKey();
  let cellSize = "";
  let cellsChanged = false;
  let frame: number | null = null;
  const queueFit = () => {
    // Fitting changes the screen itself. Wait for the next frame so observing
    // its size cannot cause a ResizeObserver delivery loop.
    if (frame !== null) return;
    frame = window.requestAnimationFrame(() => {
      frame = null;
      if (!cellsChanged && sizeKey() === fittedSize) return;
      cellsChanged = false;
      onResize();
      // Ignore the size change caused by this fit. Fractional pixel rounding
      // can otherwise make FitAddon alternate between two row counts forever.
      fittedSize = sizeKey();
    });
  };
  const observer = new ResizeObserver(() => {
    if (sizeKey() !== fittedSize) queueFit();
  });
  observer.observe(host);
  // Renderer fallback, font loading and DPI changes can change the cell size
  // without resizing the host. All rows must still fit in the visible pane.
  if (screen !== null) observer.observe(screen);
  // A fit made before xterm measured a newly applied font keeps the old rows, and the
  // screen box it leaves behind may never change again. Fit again once xterm reports
  // the cells it measured; its own row changes alter only the canvas, not the cells.
  const cellListener = view.onDimensionsChange(({ css }) => {
    const measured = `${css.cell.width}:${css.cell.height}`;
    if (measured === cellSize) return;
    cellSize = measured;
    cellsChanged = true;
    queueFit();
  });
  return () => {
    observer.disconnect();
    cellListener.dispose();
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
