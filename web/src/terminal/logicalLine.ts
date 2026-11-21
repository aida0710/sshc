import type { IBufferCell, IBufferCellPosition } from "@xterm/xterm";

// LogicalLine は、画面の何行かに分けて出された1続きの文字。リンクは、行ごとではなくこの文字から探す。
// starts と ends は、text の UTF-16 の1単位ごとに、その文字のセルの左端と右端（xterm のリンクの範囲と同じく、
// 桁も行も1から数える）。boundaries は、プログラムが改行して分けた行の変わり目の、text の中の位置。
export type LogicalLine = {
  text: string;
  starts: IBufferCellPosition[];
  ends: IBufferCellPosition[];
  boundaries: number[];
};

export function newLogicalLine(): LogicalLine {
  return { text: "", starts: [], ends: [], boundaries: [] };
}

// appendCell は、バッファの row 行 column 桁（どちらも0から数える）のセルの文字を足す。空のセルは空白として足す。
export function appendCell(logical: LogicalLine, cell: IBufferCell, { row, column }: { row: number; column: number }): void {
  const characters = cell.getChars() || " ";
  // xterm の位置はセルで数え、見つけたリンクの位置は UTF-16 の単位で数える。
  for (let unit = 0; unit < characters.length; unit += 1) {
    logical.starts.push({ x: column + 1, y: row + 1 });
    logical.ends.push({ x: column + cell.getWidth(), y: row + 1 });
  }
  logical.text += characters;
}
