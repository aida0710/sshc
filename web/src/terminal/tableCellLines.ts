import type { IBuffer, IBufferLine } from "@xterm/xterm";
import { appendCell, newLogicalLine, type LogicalLine } from "./logicalLine";

// 表のセルの中で折り返された文字を、セルごとに1続きにする。Claude Code はスマホの幅で表を出すと、セルの幅で
// URL を折る。行をそのまま読むと、URL は1行ずつに切れ、切れ端がとなりの行の文字とは別のリンクになる。
//
// つなぐのは、罫線の桁が同じ行のまとまりが、表の1つの行を何行かに分けて出したものだと分かるときだけにする。
// Claude Code の表は、表の行ごとに横の罫線で区切るので、まとまりが表の1つの行になる。psql のように横の罫線の
// 無い表では、まとまりが表の全部の行になる。そこで上下のセルをつなぐと、別の行の URL がつながる。

// セルを区切る縦の罫線。
const verticalBorder = /^[│┃║]$/u;
// 1つの行のまとまりとして読む行の上限。罫線のそろった長い出力で、1回の検索がスクロールバックを読み続けない
// ようにする。これより長いまとまりは、つながない。
const maxEntryRows = 32;
// 続きの行がスキームで始まるなら、折り返しではなく別の URL である。
const startsWithScheme = /^[a-z][a-z0-9+.-]*:\/\//iu;

// Cell は、表の行の中の1つのセル: 左右の罫線の桁（0から数える）。
type Cell = { left: number; right: number };
// Printed は、セルの中で文字のある桁の範囲。
type Printed = { first: number; last: number };

// tableCells は、罫線で始まり罫線で終わる行のセルを返す。それ以外の行は表の行ではなく、null。tree の出力の
// ように罫線で始まるだけの行は、ふつうの行として読む。
function tableCells(line: IBufferLine | undefined): Cell[] | null {
  if (!line) return null;
  const borders: number[] = [];
  let first = -1;
  let last = -1;
  for (let column = 0; column < line.length; column += 1) {
    const characters = line.getCell(column)?.getChars() ?? "";
    if (characters.trim() === "") continue;
    if (first < 0) first = column;
    last = column;
    if (verticalBorder.test(characters)) borders.push(column);
  }
  if (borders.length < 2 || borders[0] !== first || borders[borders.length - 1] !== last) return null;
  return borders.slice(1).map((right, index) => ({ left: borders[index]!, right }));
}

function sameCells(cells: Cell[], others: Cell[] | null): boolean {
  return others !== null && others.length === cells.length && others.every((other, index) => other.left === cells[index]!.left && other.right === cells[index]!.right);
}

// printedColumns は、セルの中で文字のある桁の範囲。空のセルは null。全角の文字の右半分のセルは文字のある桁に数える。
function printedColumns(line: IBufferLine, { left, right }: Cell): Printed | null {
  let first = -1;
  let last = -1;
  for (let column = left + 1; column < right; column += 1) {
    const cell = line.getCell(column);
    if (!cell || (cell.getWidth() !== 0 && cell.getChars().trim() === "")) continue;
    if (first < 0) first = column;
    last = column;
  }
  return first < 0 ? null : { first, last };
}

// fillsCell は、セルの文字が右の罫線の手前（余白は1桁まで）まで埋まっているかを返す。埋まっている行だけが、
// 次の行へ折り返されている。
function fillsCell(printed: Printed, cell: Cell): boolean {
  return printed.last >= cell.right - 2;
}

// sameBorderRows は、row 行を含む、罫線の桁が同じ行のまとまりの最初と最後の行を返す。上限より長ければ null。
function sameBorderRows(buffer: IBuffer, row: number, cells: Cell[]): { first: number; last: number } | null {
  let first = row;
  let last = row;
  while (first > 0 && sameCells(cells, tableCells(buffer.getLine(first - 1)))) {
    first -= 1;
    if (last - first >= maxEntryRows) return null;
  }
  while (last + 1 < buffer.length && sameCells(cells, tableCells(buffer.getLine(last + 1)))) {
    last += 1;
    if (last - first >= maxEntryRows) return null;
  }
  return { first, last };
}

// wrappedCell は、まとまりの中のセル cell の文字のある行を返す。セルが1続きの文字を折り返したものでなければ
// null: 文字のある行が離れている、端まで埋まらずに次の行へ続いている、続きがスキームで始まる、のどれか。
function wrappedCell(buffer: IBuffer, rows: { first: number; last: number }, cell: Cell): { row: number; printed: Printed }[] | null {
  const printedRows: { row: number; printed: Printed }[] = [];
  for (let row = rows.first; row <= rows.last; row += 1) {
    const line = buffer.getLine(row)!;
    const printed = printedColumns(line, cell);
    if (printed === null) continue;
    const previous = printedRows[printedRows.length - 1];
    if (previous !== undefined) {
      if (previous.row !== row - 1 || !fillsCell(previous.printed, cell)) return null;
      if (startsWithScheme.test(line.translateToString(false, printed.first, printed.last + 1))) return null;
    }
    printedRows.push({ row, printed });
  }
  return printedRows;
}

function cellText(buffer: IBuffer, printedRows: { row: number; printed: Printed }[]): LogicalLine {
  const logical = newLogicalLine();
  for (const [index, { row, printed }] of printedRows.entries()) {
    if (index > 0) logical.boundaries.push(logical.text.length);
    const line = buffer.getLine(row)!;
    for (let column = printed.first; column <= printed.last; column += 1) {
      const bufferCell = line.getCell(column);
      if (bufferCell && bufferCell.getWidth() !== 0) appendCell(logical, bufferCell, { row, column });
    }
  }
  return logical;
}

// tableCellLines は、row 行（0から数える）が表の行のとき、その行の各セルの文字を返す。表の行でなければ null。
// 行のまとまりが表の1つの行を分けて出したものなら、セルの中の折り返しをつないだ文字を返す。そう分かるのは、
// 列が2つ以上あり、どのセルも1続きの文字を折り返した形のとき。列が1つの枠は、長い行が端まで届くだけで
// 折り返しと見分けられないので、つながない。
export function tableCellLines(buffer: IBuffer, row: number): LogicalLine[] | null {
  const cells = tableCells(buffer.getLine(row));
  if (cells === null) return null;
  const onThisRow = (cell: Cell): LogicalLine => {
    const printed = printedColumns(buffer.getLine(row)!, cell);
    return printed === null ? newLogicalLine() : cellText(buffer, [{ row, printed }]);
  };
  const rows = cells.length < 2 ? null : sameBorderRows(buffer, row, cells);
  if (rows === null) return cells.map(onThisRow);
  const wrapped = cells.map((cell) => wrappedCell(buffer, rows, cell));
  if (wrapped.some((printedRows) => printedRows === null)) return cells.map(onThisRow);
  return wrapped.map((printedRows) => cellText(buffer, printedRows!));
}
