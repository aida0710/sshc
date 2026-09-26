import type { IBuffer, IBufferRange, IBufferCellPosition } from "@xterm/xterm";
import { findTerminalLinks, type TerminalLinkMatch } from "./links";

// Bound work during mouse movement even when a program prints without newlines.
const maxLogicalLineCells = 65536;

type PositionedLink = { match: TerminalLinkMatch; range: IBufferRange };

export function findBufferLinks(buffer: IBuffer, lineNumber: number, remote: boolean): PositionedLink[] {
  let firstRow = lineNumber - 1;
  let cellCount = 0;
  while (firstRow > 0 && buffer.getLine(firstRow)?.isWrapped) {
    cellCount += buffer.getLine(firstRow)?.length ?? 0;
    if (cellCount > maxLogicalLineCells) return [];
    firstRow -= 1;
  }

  let text = "";
  const starts: IBufferCellPosition[] = [];
  const ends: IBufferCellPosition[] = [];
  cellCount = 0;
  for (let row = firstRow; row < buffer.length; row += 1) {
    const line = buffer.getLine(row);
    if (!line || (row > firstRow && !line.isWrapped)) break;
    cellCount += line.length;
    if (cellCount > maxLogicalLineCells) return [];
    for (let column = 0; column < line.length; column += 1) {
      const cell = line.getCell(column);
      if (!cell || cell.getWidth() === 0) continue;
      // A wide character moves to the next row when only one cell remains.
      // The empty padding cell is not a space in the logical URL.
      const nextLine = column === line.length - 1 ? buffer.getLine(row + 1) : undefined;
      if (cell.getChars() === "" && nextLine?.isWrapped && nextLine.getCell(0)?.getWidth() === 2) continue;
      const characters = cell.getChars() || " ";
      // xterm coordinates count cells; JS match offsets count UTF-16 code units.
      for (let unit = 0; unit < characters.length; unit += 1) {
        starts.push({ x: column + 1, y: row + 1 });
        ends.push({ x: column + cell.getWidth(), y: row + 1 });
      }
      text += characters;
    }
  }

  return findTerminalLinks(text, remote).flatMap((match) => {
    const start = starts[match.start];
    const end = ends[match.end - 1];
    if (!start || !end || lineNumber < start.y || lineNumber > end.y) return [];
    return [{ match, range: { start, end } }];
  });
}
