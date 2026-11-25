// The grid an ASCII art is drawn on: a canvas cut into cells the size of one character, each holding at most one
// character in one of the page's colours (the ink, the accent, or one of the terminal's) at some opacity.
//
// An art puts the characters of a frame (putCharacter) and the grid draws only the cells that differ from the
// frame on the canvas (presentFrame). Drawing a character costs far more than working out the frame, and from one
// frame to the next most cells stay the same: the lines of the network, the slowly drifting landscape.

// The colours an art draws with: the page's ink and accent, and the terminal's colours, so that a coloured art
// reads in every theme as the terminal's own output does.
export const asciiColorNames = ["ink", "accent", "red", "green", "yellow", "blue", "magenta", "cyan"] as const;
export type AsciiColor = (typeof asciiColorNames)[number];
export type AsciiColors = Readonly<Record<AsciiColor, string>>;

export type AsciiGrid = {
  readonly context: CanvasRenderingContext2D;
  // width and height are in CSS pixels.
  readonly width: number;
  readonly height: number;
  readonly cellWidth: number;
  readonly cellHeight: number;
  // baseline is how far below a cell's top its character's baseline lies.
  readonly baseline: number;
  readonly columns: number;
  readonly rows: number;
  // intensity multiplies every character's opacity: a small sample (Settings) is drawn darker to be seen.
  readonly intensity: number;
  readonly colors: AsciiColors;
  // The frame on the canvas and the frame being put, a character code (0 for none), a colour (its index in
  // asciiColorNames) and an opacity level for each cell.
  readonly shown: CellFrame;
  readonly next: CellFrame;
};

type CellFrame = { characters: Uint16Array; colors: Uint8Array; levels: Uint8Array };

// The terminal's own font (index.css loads it), so the art reads as part of the terminal.
export const asciiFontFamily = '"JetBrains Mono", ui-monospace, monospace';
// A cell is a little taller than the font, so the rows do not touch, and never shorter than the font's tallest
// characters reach: a character that hung out of its cell would leave a mark below when the cell is cleared.
const cellHeightPerFontSize = 1.3;
// A monospace character is about 0.6 of its font size wide; used where the canvas cannot measure (tests).
const fallbackCellWidthPerFontSize = 0.6;
// Opacity is kept in this many steps, so that a value that drifts a little does not redraw its cell every frame.
const opacityLevels = 15;
const colorIndexes = Object.fromEntries(asciiColorNames.map((name, index) => [name, index])) as Record<AsciiColor, number>;

// shadeRamp goes from no ink to the most ink in one cell.
const shadeRamp = " .:-=+*#%@";

export function createAsciiGrid(canvas: HTMLCanvasElement, fontSize: number, intensity: number, colors: AsciiColors): AsciiGrid | null {
  const context = canvas.getContext("2d");
  const width = canvas.clientWidth;
  const height = canvas.clientHeight;
  if (context === null || width === 0 || height === 0) return null;
  const ratio = window.devicePixelRatio || 1;
  canvas.width = Math.round(width * ratio);
  canvas.height = Math.round(height * ratio);
  context.setTransform(ratio, 0, 0, ratio, 0, 0);
  context.font = `${fontSize}px ${asciiFontFamily}`;
  context.textBaseline = "alphabetic";
  // Cells are whole device pixels, so clearing one cell does not touch its neighbours, rounded up so that a cell
  // holds its character.
  const snap = (value: number) => Math.max(1, Math.ceil(value * ratio)) / ratio;
  const metrics = context.measureText("M");
  const cellWidth = snap(metrics.width || fontSize * fallbackCellWidthPerFontSize);
  // The font's box: from its highest ascender to its lowest descender. The canvas of a test does not measure it.
  const ascent = metrics.fontBoundingBoxAscent || fontSize;
  const descent = metrics.fontBoundingBoxDescent || fontSize * (cellHeightPerFontSize - 1);
  const cellHeight = snap(Math.max(fontSize * cellHeightPerFontSize, ascent + descent));
  const baseline = (cellHeight - ascent - descent) / 2 + ascent;
  const columns = Math.ceil(width / cellWidth);
  const rows = Math.ceil(height / cellHeight);
  return {
    context, width, height, cellWidth, cellHeight, baseline, columns, rows, intensity, colors,
    shown: emptyFrame(columns * rows), next: emptyFrame(columns * rows),
  };
}

function emptyFrame(cells: number): CellFrame {
  return { characters: new Uint16Array(cells), colors: new Uint8Array(cells), levels: new Uint8Array(cells) };
}

// beginFrame empties the frame being put.
export function beginFrame(grid: AsciiGrid): void {
  grid.next.characters.fill(0);
}

// putCharacter puts a character into the frame being put; a later one in the same cell takes its place.
export function putCharacter(grid: AsciiGrid, column: number, row: number, character: string, color: AsciiColor, opacity: number): void {
  if (column < 0 || row < 0 || column >= grid.columns || row >= grid.rows || character === " ") return;
  const level = Math.round(Math.min(1, opacity * grid.intensity) * opacityLevels);
  if (level === 0) return;
  const cell = row * grid.columns + column;
  grid.next.characters[cell] = character.charCodeAt(0);
  grid.next.colors[cell] = colorIndexes[color];
  grid.next.levels[cell] = level;
}

// presentFrame draws the cells whose character, colour or opacity changed since the frame on the canvas.
export function presentFrame(grid: AsciiGrid): void {
  const { context, shown, next, columns, cellWidth, cellHeight, baseline } = grid;
  const fills = asciiColorNames.map((name) => grid.colors[name]);
  for (let cell = 0; cell < next.characters.length; cell++) {
    const character = next.characters[cell] ?? 0;
    const color = next.colors[cell] ?? 0;
    const level = next.levels[cell] ?? 0;
    if (character === shown.characters[cell] && (character === 0 || (color === shown.colors[cell] && level === shown.levels[cell]))) continue;
    const x = (cell % columns) * cellWidth;
    const y = Math.floor(cell / columns) * cellHeight;
    context.clearRect(x, y, cellWidth, cellHeight);
    shown.characters[cell] = character;
    shown.colors[cell] = color;
    shown.levels[cell] = level;
    if (character === 0) continue;
    context.globalAlpha = level / opacityLevels;
    context.fillStyle = fills[color] ?? grid.colors.ink;
    context.fillText(String.fromCharCode(character), x, y + baseline);
  }
}

// shadeCharacter is the character for an amount of ink in [0, 1].
export function shadeCharacter(amount: number): string {
  return shadeRamp[Math.max(0, Math.min(shadeRamp.length - 1, Math.floor(amount * shadeRamp.length)))] ?? " ";
}

// resolveAsciiColors reads the colours an art draws with from the page, as it is drawn now.
export function resolveAsciiColors(): AsciiColors {
  const css: AsciiColors = {
    ink: "var(--ui-ink)", accent: "var(--ui-accent)",
    red: "var(--ui-term-red)", green: "var(--ui-term-green)", yellow: "var(--ui-term-yellow)",
    blue: "var(--ui-term-blue)", magenta: "var(--ui-term-magenta)", cyan: "var(--ui-term-cyan)",
  };
  return Object.fromEntries(asciiColorNames.map((name) => [name, resolveCssColor(css[name])])) as AsciiColors;
}

// resolveCssColor turns a CSS colour (with var()) into the rgb() a canvas reads, as the page draws it now.
function resolveCssColor(css: string): string {
  const probe = document.createElement("span");
  probe.style.color = css;
  probe.style.display = "none";
  document.body.append(probe);
  const color = getComputedStyle(probe).color;
  probe.remove();
  return color;
}
