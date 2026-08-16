// 指で端末を流す。
//
// **xterm はこれを持っていない。** スクロールするのは .xterm-viewport だが、
// それは絶対配置で下に敷かれており、上に .xterm-screen が乗っている——指が
// 触れるのは常に上の層なので、ブラウザは何も流すものを見つけられない。
// ホイールは xterm が自分で拾って scrollLines を呼んでおり、ここでやるのは
// それと同じことを触れる画面のために書くだけである。
//
// **preventDefault しない。** 止めれば長押しからの範囲選択も一緒に殺す。
// 指を引く操作は流し、長押しは端末に渡す——後者は OS の仕事である。

export type Scroller = { rows: number; scrollLines(amount: number): void };

/**
 * 1 行に満たない動きを溜める。
 *
 * <p>行単位でしか流せないので、切り捨てた端数を捨てると、ゆっくり引いた指は
 * いつまでも何も起こさない。**溜めて、超えた分だけ渡す。**
 */
export function newTouchScroll(view: Scroller, cellHeight: () => number) {
  let last = 0;
  let carried = 0;

  return {
    start(y: number) {
      last = y;
      carried = 0;
    },
    /** 引いた距離を行数に変え、渡した行数を返す。 */
    move(y: number): number {
      const cell = cellHeight();
      if (cell <= 0) return 0;
      // 上へ引けば内容は先へ進む。符号はホイールと同じ向きに揃える。
      carried += (last - y) / cell;
      last = y;
      const lines = Math.trunc(carried);
      if (lines === 0) return 0;
      carried -= lines;
      view.scrollLines(lines);
      return lines;
    },
  };
}

/** cellHeight は、描かれている行の高さを実測する。 */
export function measuredCellHeight(container: HTMLElement, rows: number): number {
  if (rows <= 0) return 0;
  return container.clientHeight / rows;
}
