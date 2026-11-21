import type { IBuffer, IBufferLine, IBufferRange } from "@xterm/xterm";
import { findTerminalLinks, type TerminalLinkMatch } from "./links";
import { appendCell, newLogicalLine, type LogicalLine } from "./logicalLine";
import { previewableNamePattern } from "./pathLinks";
import { tableCellLines } from "./tableCellLines";

// Bound work during mouse movement even when a program prints without newlines.
const maxLogicalLineCells = 65536;
// Program-inserted newlines carry no wrap metadata. Limit the inferred chain
// to 32 rows so aligned output cannot turn a hover into a scrollback scan.
const maxHardWrappedRows = 32;
const pathContinuationCharacter = /^[\p{L}\p{N}\p{M}_./\\~%\-]/u;
// URL の途中に出る記号。プログラムが URL を記号の手前で折ると、続きの行はこの記号で始まる
// （…?redirect_uri の次の行が =http%3A… で始まる）。
const urlMark = /^[=&?#+:;,@!$()*]/u;
// Characters of the middle of a URL that the words of a sentence rarely hold.
const urlMiddleCharacter = /[/?#&=%+~]/u;

type PositionedLink = { match: TerminalLinkMatch; range: IBufferRange };

// startsContinuation は、行の column からの文字が、前の行の続きの始まりに見えるかを返す。記号で始まる行は、
// すぐ後ろに文字が続くときだけ続きと読む。記号のあとが空白の行は、見出し（# …）や箇条書き（+ …、* …）である。
function startsContinuation(line: IBufferLine, column: number): boolean {
  const first = line.getCell(column)?.getChars() ?? "";
  if (pathContinuationCharacter.test(first)) return true;
  return urlMark.test(first) && /^\S/u.test(line.getCell(column + 1)?.getChars() ?? "");
}

function continuationColumn(buffer: IBuffer, row: number): number | null {
  const line = buffer.getLine(row);
  const previous = buffer.getLine(row - 1);
  if (!line || !previous) return null;
  if (line.isWrapped) return 0;
  let column = 0;
  while (column < line.length && /^\s*$/u.test(line.getCell(column)?.getChars() ?? "")) column += 1;
  // Indentation is evidence of a program's continuation, but its width is
  // program-dependent. Bullets, prompts and blank rows are separate output.
  if (column === 0 || !startsContinuation(line, column)) return null;
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
  // A complete file name at the edge is a separate entry, not a truncated name.
  return !previewableNamePattern.test(prefix) && !/["'`\s]$/u.test(prefix);
}

// urlContinuesAcross says whether a URL a program broke at the edge of the terminal goes on after the break.
// A URL that only happens to end at the edge must not take the next word: the part after the break has to look
// like the middle of a URL, or fill its row and go on past the next break.
function urlContinuesAcross(match: TerminalLinkMatch, boundary: number, nextBoundary: number | undefined): boolean {
  if (match.kind !== "url" || match.start >= boundary || match.end <= boundary) return false;
  if (nextBoundary !== undefined && match.end > nextBoundary) return true;
  return urlMiddleCharacter.test(match.text.slice(boundary - match.start, (nextBoundary ?? match.end) - match.start));
}

function linksAcrossBoundaries(text: string, boundaries: number[], allowWindowsPaths: boolean): TerminalLinkMatch[] {
  const matches = findTerminalLinks(text, allowWindowsPaths);
  const rejected = boundaries.find((boundary) => !matches.some((match) => {
    if (urlContinuesAcross(match, boundary, boundaries.find((offset) => offset > boundary))) return true;
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

// bufferRangeText returns the text that a link range covers, read by cells: a
// slice of the row's string is shifted by one for every two-cell character
// before the link. The range continues across wrapped rows.
export function bufferRangeText(buffer: IBuffer, range: IBufferRange): string {
  let text = "";
  for (let row = range.start.y; row <= range.end.y; row += 1) {
    const line = buffer.getLine(row - 1);
    if (!line) return "";
    const start = row === range.start.y ? range.start.x - 1 : 0;
    const end = row === range.end.y ? range.end.x : line.length;
    text += line.translateToString(false, start, end);
  }
  return text;
}

// wrappedRowsLine は、lineNumber の行を含む、折り返された行のつながりを1続きの文字にする。xterm が折り返した行
// （isWrapped）と、プログラムが右端で改行して字下げした続きをつなぐ。長すぎるつながりは null。
function wrappedRowsLine(buffer: IBuffer, lineNumber: number): LogicalLine | null {
  let firstRow = lineNumber - 1;
  let cellCount = 0;
  let hardWrappedRows = 1;
  while (firstRow > 0 && continuationColumn(buffer, firstRow) !== null) {
    cellCount += buffer.getLine(firstRow)?.length ?? 0;
    if (!buffer.getLine(firstRow)?.isWrapped) hardWrappedRows += 1;
    if (cellCount > maxLogicalLineCells || hardWrappedRows > maxHardWrappedRows) return null;
    firstRow -= 1;
  }

  const logical = newLogicalLine();
  cellCount = 0;
  hardWrappedRows = 1;
  for (let row = firstRow; row < buffer.length; row += 1) {
    const line = buffer.getLine(row);
    const firstColumn = row === firstRow ? 0 : continuationColumn(buffer, row);
    if (!line || firstColumn === null) break;
    cellCount += line.length;
    if (row > firstRow && !line.isWrapped) {
      hardWrappedRows += 1;
      logical.boundaries.push(logical.text.length);
    }
    if (cellCount > maxLogicalLineCells || hardWrappedRows > maxHardWrappedRows) return null;
    for (let column = firstColumn; column < line.length; column += 1) {
      const cell = line.getCell(column);
      if (!cell || cell.getWidth() === 0) continue;
      // Padding before a wide continuation is not part of the printed path.
      const nextColumn = column === line.length - 1 ? continuationColumn(buffer, row + 1) : null;
      if (cell.getChars() === "" && nextColumn !== null && buffer.getLine(row + 1)?.getCell(nextColumn)?.getWidth() === 2) continue;
      appendCell(logical, cell, { row, column });
    }
  }
  return logical;
}

// linksOnRow は、1続きの文字の中のリンク matches のうち、lineNumber の行にかかるものを、その行での範囲つきで返す。
function linksOnRow({ starts, ends, boundaries }: LogicalLine, matches: TerminalLinkMatch[], lineNumber: number): PositionedLink[] {
  return matches.flatMap((match) => {
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

export function findBufferLinks(buffer: IBuffer, lineNumber: number, allowWindowsPaths = false): PositionedLink[] {
  // 表の行は、セルごとに読む。行をそのまま読むと、セルの中で折り返された URL が1行ずつに切れ、となりのセルの
  // 文字ともつながる。
  // セルの中の変わり目は、同じ文字の折り返しだと分かっている（tableCellLines.ts）。行のつながりの変わり目は
  // 字下げからの推測なので、リンクが変わり目をまたいでよいかをリンクごとに確かめる。
  const cells = tableCellLines(buffer, lineNumber - 1);
  if (cells !== null) return cells.flatMap((cell) => linksOnRow(cell, findTerminalLinks(cell.text, allowWindowsPaths), lineNumber));
  const rows = wrappedRowsLine(buffer, lineNumber);
  return rows === null ? [] : linksOnRow(rows, linksAcrossBoundaries(rows.text, rows.boundaries, allowWindowsPaths), lineNumber);
}
