// 端末の中身を、ただの文字列として取り出す。
//
// **なぜ要るのか。** 触れる画面で範囲を選ぶ手立てが他に無いからである。
// xterm の選択はマウスのためのもので、押して・引いて・離すの 3 つが指には
// 無い。そして OS が持っている長押しからの選択も使えない——xterm は自分の
// mousedown で無条件に preventDefault を呼んでおり、長押しが生む mousedown も
// そこで潰れる。CSS で user-select を戻しても、選択は始まらない。
//
// 残る道は、xterm の外に同じ文字を置いて、そちらを OS に選ばせることである。

export type BufferLine = { translateToString(trimRight?: boolean): string };
export type ReadableBuffer = { length: number; getLine(index: number): BufferLine | undefined };

/**
 * bufferText は、スクロールバックを含む全行を 1 つの文字列にする。
 *
 * <p>右端の空白は落とす。端末の行は常に桁数いっぱいの長さを持つので、落とさな
 * ければ、選んで貼り付けたものの行末に空白が何十個も付いてくる。
 *
 * <p>末尾の空行も落とす。まだ何も書かれていない行が数十行あるのが端末の常態
 * であり、それをそのまま渡すと、開いた面の大半が空白になる。
 */
export function bufferText(buffer: ReadableBuffer): string {
  const lines: string[] = [];
  for (let index = 0; index < buffer.length; index += 1) {
    lines.push(buffer.getLine(index)?.translateToString(true) ?? "");
  }
  while (lines.length > 0 && lines[lines.length - 1] === "") lines.pop();
  return lines.join("\n");
}
