import type { IBufferCell, IBufferLine, ITheme } from "@xterm/xterm";

// The 16 colours a program names by number (SGR 30–37 and 90–97), as the theme names them.
const themeColourNames = [
  "black", "red", "green", "yellow", "blue", "magenta", "cyan", "white",
  "brightBlack", "brightRed", "brightGreen", "brightYellow", "brightBlue", "brightMagenta", "brightCyan", "brightWhite",
] as const satisfies readonly (keyof ITheme)[];
// The six levels of each primary in the 6×6×6 colour cube of a 256-colour palette (indices 16–231), as xterm defines it.
const cubeLevels = [0, 95, 135, 175, 215, 255] as const;
// The grey ramp (indices 232–255) starts at this level and steps by greyStep.
const greyStart = 8;
const greyStep = 10;

function hex(red: number, green: number, blue: number): string {
  return `#${[red, green, blue].map((value) => value.toString(16).padStart(2, "0")).join("")}`;
}

// paletteColour is the colour of a 256-colour palette index: the theme's 16 colours, the colour cube and the greys.
export function paletteColour(index: number, theme: ITheme): string | undefined {
  if (index < themeColourNames.length) return theme[themeColourNames[index]!];
  if (index < 232) {
    const cube = index - 16;
    return hex(cubeLevels[Math.floor(cube / 36)]!, cubeLevels[Math.floor(cube / 6) % 6]!, cubeLevels[cube % 6]!);
  }
  const grey = greyStart + (index - 232) * greyStep;
  return hex(grey, grey, grey);
}

function rgbColour(value: number): string {
  return hex((value >> 16) & 0xff, (value >> 8) & 0xff, value & 0xff);
}

function foreground(cell: IBufferCell, theme: ITheme): string | undefined {
  if (cell.isFgRGB()) return rgbColour(cell.getFgColor());
  if (cell.isFgPalette()) return paletteColour(cell.getFgColor(), theme);
  return undefined;
}

function background(cell: IBufferCell, theme: ITheme): string | undefined {
  if (cell.isBgRGB()) return rgbColour(cell.getBgColor());
  if (cell.isBgPalette()) return paletteColour(cell.getBgColor(), theme);
  return undefined;
}

// drawEdgeRow fills element with one line of the terminal's buffer: each cell in a box of the cells' width, so the
// line keeps the columns of the rows the terminal draws (xterm.js's WebGL renderer puts every cell on whole pixels,
// which text laid out by the font alone would drift from). It keeps the colours, bold, italic, dim and underline; it is
// shown only for the moment a finger moves the rows by part of a row.
export function drawEdgeRow(element: HTMLElement, line: IBufferLine | undefined, { cols, cellWidth, theme }: {
  cols: number;
  cellWidth: number;
  theme: ITheme;
}): void {
  element.replaceChildren();
  if (line === undefined) return;
  const document = element.ownerDocument;
  const reused = line.getCell(0);
  if (reused === undefined) return;
  // Blank cells at the end of the line draw nothing.
  let end = Math.min(cols, line.length);
  while (end > 0) {
    const cell = line.getCell(end - 1, reused);
    if (cell === undefined || (cell.getChars().trim() !== "" || !cell.isBgDefault())) break;
    end -= 1;
  }
  for (let column = 0; column < end; column += 1) {
    const cell = line.getCell(column, reused);
    if (cell === undefined) break;
    const width = cell.getWidth();
    // The second half of a wide character belongs to the first.
    if (width === 0) continue;
    let colour = foreground(cell, theme);
    let fill = background(cell, theme);
    if (cell.isInverse() !== 0) [colour, fill] = [fill ?? theme.background, colour ?? theme.foreground];
    const box = document.createElement("span");
    box.textContent = cell.getChars() === "" ? " " : cell.getChars();
    box.style.display = "inline-block";
    box.style.width = `${cellWidth * width}px`;
    box.style.overflow = "visible";
    if (cell.isInvisible() !== 0) box.style.visibility = "hidden";
    if (colour !== undefined) box.style.color = colour;
    if (fill !== undefined) box.style.backgroundColor = fill;
    if (cell.isBold() !== 0) box.style.fontWeight = "bold";
    if (cell.isItalic() !== 0) box.style.fontStyle = "italic";
    if (cell.isDim() !== 0) box.style.opacity = "0.5";
    if (cell.isUnderline() !== 0) box.style.textDecoration = "underline";
    element.appendChild(box);
  }
}
