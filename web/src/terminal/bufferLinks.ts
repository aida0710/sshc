import type { IBuffer, IBufferRange, IBufferCellPosition } from "@xterm/xterm";
import { findTerminalLinks, type TerminalLinkMatch } from "./links";
import { imageNamePattern } from "./pathLinks";

// Bound work during mouse movement even when a program prints without newlines.
const maxLogicalLineCells = 65536;
// Program-inserted newlines carry no wrap metadata. Limit the inferred chain
// to 32 rows so aligned output cannot turn a hover into a scrollback scan.
const maxHardWrappedRows = 32;
const pathContinuationCharacter = /^[\p{L}\p{N}\p{M}_./\\~%\-]/u;

type PositionedLink = { match: TerminalLinkMatch; range: IBufferRange };

function continuationColumn(buffer: IBuffer, row: number): number | null {
  const line = buffer.getLine(row);
  const previous = buffer.getLine(row - 1);
  if (!line || !previous) return null;
  if (line.isWrapped) return 0;
  let column = 0;
  while (column < line.length && /^\s*$/u.test(line.getCell(column)?.getChars() ?? "")) column += 1;
  // Indentation is evidence of a program's continuation, but its width is
  // program-dependent. Bullets, prompts and blank rows are separate output.
  if (column === 0 || !pathContinuationCharacter.test(line.getCell(column)?.getChars() ?? "")) return null;
  const last = previous.getCell(previous.length - 1);
  if (!last) return null;
  if (last.getWidth() === 0 || /\S/u.test(last.getChars())) return column;
  // A two-cell character cannot fit in the one remaining column. Accept only
  // an empty padding cell here, not a printed space or a generally short row.
  if (last.getChars() === "" && line.getCell(column)?.getWidth() === 2 &&
    /\S/u.test(previous.translateToString(false, previous.length - 2, previous.length - 1))) return column;
  return null;
}

function crossesPathBoundary(match: TerminalLinkMatch, boundary: number): boolean {
  if (match.kind === "url" || match.start >= boundary || match.end <= boundary) return false;
  const prefix = match.text.slice(0, boundary - match.start);
  // A complete image at the edge is a separate entry, not a truncated name.
  return !imageNamePattern.test(prefix) && !/["'`\s]$/u.test(prefix);
}

function linksAcrossBoundaries(text: string, boundaries: number[], allowWindowsPaths: boolean): TerminalLinkMatch[] {
  const matches = findTerminalLinks(text, allowWindowsPaths);
  const rejected = boundaries.find((boundary) => !matches.some((match) => {
    if (!crossesPathBoundary(match, boundary)) return false;
    const prefix = text.slice(match.start, boundaries.find((offset) => offset > match.start) ?? boundary);
    // A slash before the break distinguishes paths from the last word of
    // prose. Bare image names must start a line or bullet, possibly quoted,
    // so an unrelated following image cannot turn that last word into a path.
    return /[/\\]/u.test(prefix) || /^\s*(?:[●•]\s+)?["'`]?$/u.test(text.slice(0, match.start));
  }));
  if (rejected === undefined) return matches;
  // Parse rejected joins independently. This also restores ordinary links on
  // either side without returning fragments of any accepted, joined path.
  return [
    ...linksAcrossBoundaries(text.slice(0, rejected), boundaries.filter((offset) => offset < rejected), allowWindowsPaths),
    ...linksAcrossBoundaries(text.slice(rejected), boundaries.filter((offset) => offset > rejected).map((offset) => offset - rejected), allowWindowsPaths)
      .map((match) => ({ ...match, start: match.start + rejected, end: match.end + rejected })),
  ];
}

export function findBufferLinks(buffer: IBuffer, lineNumber: number, allowWindowsPaths = false): PositionedLink[] {
  let firstRow = lineNumber - 1;
  let cellCount = 0;
  let hardWrappedRows = 1;
  while (firstRow > 0 && continuationColumn(buffer, firstRow) !== null) {
    cellCount += buffer.getLine(firstRow)?.length ?? 0;
    if (!buffer.getLine(firstRow)?.isWrapped) hardWrappedRows += 1;
    if (cellCount > maxLogicalLineCells || hardWrappedRows > maxHardWrappedRows) return [];
    firstRow -= 1;
  }

  let text = "";
  const starts: IBufferCellPosition[] = [];
  const ends: IBufferCellPosition[] = [];
  const boundaries: number[] = [];
  cellCount = 0;
  hardWrappedRows = 1;
  for (let row = firstRow; row < buffer.length; row += 1) {
    const line = buffer.getLine(row);
    const firstColumn = row === firstRow ? 0 : continuationColumn(buffer, row);
    if (!line || firstColumn === null) break;
    cellCount += line.length;
    if (row > firstRow && !line.isWrapped) {
      hardWrappedRows += 1;
      boundaries.push(text.length);
    }
    if (cellCount > maxLogicalLineCells || hardWrappedRows > maxHardWrappedRows) return [];
    for (let column = firstColumn; column < line.length; column += 1) {
      const cell = line.getCell(column);
      if (!cell || cell.getWidth() === 0) continue;
      // Padding before a wide continuation is not part of the printed path.
      const nextColumn = column === line.length - 1 ? continuationColumn(buffer, row + 1) : null;
      if (cell.getChars() === "" && nextColumn !== null && buffer.getLine(row + 1)?.getCell(nextColumn)?.getWidth() === 2) continue;
      const characters = cell.getChars() || " ";
      // xterm coordinates count cells; JS match offsets count UTF-16 code units.
      for (let unit = 0; unit < characters.length; unit += 1) {
        starts.push({ x: column + 1, y: row + 1 });
        ends.push({ x: column + cell.getWidth(), y: row + 1 });
      }
      text += characters;
    }
  }

  return linksAcrossBoundaries(text, boundaries, allowWindowsPaths).flatMap((match) => {
    let start = starts[match.start];
    let end = ends[match.end - 1];
    if (!start || !end || lineNumber < start.y || lineNumber > end.y) return [];
    if (boundaries.some((offset) => match.start < offset && match.end > offset)) {
      // xterm has one continuous range per link. Offer each printed row with
      // the same target so indentation never becomes underlined or tappable.
      start = starts.slice(match.start, match.end).find((position) => position.y === lineNumber)!;
      end = ends.slice(match.start, match.end).reverse().find((position) => position.y === lineNumber)!;
    }
    return [{ match, range: { start, end } }];
  });
}
