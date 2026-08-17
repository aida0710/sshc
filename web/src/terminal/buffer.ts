// 端末の中身を、ただの文字列として取り出す。
//
// **なぜ要るのか。** `.xterm` の中では、指の長押しから範囲選択が始まらない。
// 実機に DevTools を繋いで、同じ普通の div を中と外に置いて比べた——中では
// 選べず、外では選べる。**mousedown は関係ない**（長押しでは 1 度も発火しない）。
// user-select も pointer-events も touch-action も contextmenu の遮断も、
// 1 つずつ潰して全部空振りだった。
//
// 残る道は、xterm の外に同じ文字を置いて、そちらを OS に選ばせることである。
// ここにあるのは、その板に載せる字を取り出す一つの関数だけである。

export type BufferLine = {
  translateToString(trimRight?: boolean, startColumn?: number, endColumn?: number): string;
};
export type ReadableBuffer = { length: number; getLine(index: number): BufferLine | undefined };
/** 見えている窓。viewportY は、その窓がバッファのどこから始まるかである。 */
export type ViewportBuffer = ReadableBuffer & { viewportY: number };

/**
 * viewportText は、**いま見えている行だけ**を 1 つの文字列にする。
 *
 * <p>bufferText との違いは窓の有無である。あちらはスクロールバックごと別の面へ
 * 写すためのもので、こちらは端末の上に重ねる透明な板のためのもの——重ねる以上、
 * 見えていない行が混ざっていてはならない。
 *
 * <p>**末尾の空行を落とさない。** bufferText は落とすが、それは面いっぱいの
 * 空白を避けるためである。ここで落とすと、n 行目が n 行目の上に来なくなる
 * ——それだけがこの板の存在理由である。
 *
 * <p>endColumn に cols を渡す。行の配列は resize のあと桁数より長いまま残る
 * ことがあり、渡さないと箱の外まで字が伸びる。
 */
export function viewportText(buffer: ViewportBuffer, rows: number, cols: number): string {
  const lines: string[] = [];
  for (let row = 0; row < rows; row += 1) {
    lines.push(buffer.getLine(buffer.viewportY + row)?.translateToString(true, 0, cols) ?? "");
  }
  return lines.join("\n");
}
