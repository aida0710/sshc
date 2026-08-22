// 端末の 1 マスがどれだけの大きさで、その字がどの字体に解決したかを答える。
//
// **xterm の DOM を掻くのは、このファイルだけである。**
//
// 以前は 2 か所が別々に測っていた。どちらも「1 行の高さ」だが、導出が違う:
//
//   selectionOverlay  .xterm-screen の getBoundingClientRect().height / rows
//   TerminalView      .xterm-screen の clientHeight / rows
//
// 前者は端数を持ち、後者は整数に丸められる。境界線も padding も無いあいだは
// ほぼ同じ数が出る——**揃っていたのは偶然であって、約束ではなかった。**
// ここでは端数を持つ方に寄せた。指で流す側は 1 行に満たない動きを溜めるので、
// 丸めた高さを渡すと、ゆっくり引いた指がわずかにずれ続ける。
//
// 描画器を差し替えれば .xterm-screen も .xterm-rows も無くなる。**そのとき
// 直すのはこのファイルだけである** ——metrics.test.ts が、他所に xterm の
// セレクタが増えたら赤にする。

export type MeasurableTerminal = {
  readonly element: HTMLElement | undefined;
  readonly rows: number;
};

export type CellMetrics = {
  /** 字が描かれている面の矩形。ビューポート座標である。 */
  readonly rect: DOMRect;
  /** 1 行の高さ。端数を持つ。 */
  readonly cellHeight: number;
  /**
   * 面の字が実際に解決した字体。
   *
   * <p>**写すのであって、測り直さない。** 端末が ui-monospace をどの実物に
   * 解決したかは、こちらからは分からない。
   */
  readonly font: {
    readonly family: string;
    readonly size: string;
    readonly weight: string;
    /**
     * **字送りを決めているのは family ではなくこちらである。** xterm は
     * 「1 マスの幅 − 実測した W の幅」をここへ入れ、解決した等幅がどれで
     * あっても桁が揃うようにしている。読めば、その較正ごと写せる。
     */
    readonly letterSpacing: string;
  };
};

/** surface は、字が描かれている面である。まだ建っていなければ null。 */
function surface(view: MeasurableTerminal): HTMLElement | null {
  return view.element?.querySelector<HTMLElement>(".xterm-screen") ?? null;
}

/**
 * perRow は、面の高さを行数で割る。**割り算はここにしかない。**
 *
 * <p>行数が 0 なら 0 を返す。0 で割れば Infinity 行流れる。
 */
function perRow(height: number, rows: number): number {
  return rows <= 0 ? 0 : height / rows;
}

/**
 * measureCells は、面の矩形と 1 行の高さと字体を返す。
 *
 * <p>面がまだ建っていない、あるいは大きさが 0 のあいだは **null を返す。**
 * 0 を返さないのは、呼ぶ側が「測れなかった」と「高さが 0 だった」を区別
 * できなくなるからである。
 */
export function measureCells(view: MeasurableTerminal): CellMetrics | null {
  const screen = surface(view);
  const glyphs = view.element?.querySelector<HTMLElement>(".xterm-rows") ?? null;
  if (screen === null || glyphs === null || view.rows <= 0) return null;
  const rect = screen.getBoundingClientRect();
  if (rect.width <= 0 || rect.height <= 0) return null;
  const style = getComputedStyle(glyphs);
  return {
    rect,
    cellHeight: perRow(rect.height, view.rows),
    font: {
      family: style.fontFamily,
      size: style.fontSize,
      weight: style.fontWeight,
      letterSpacing: style.letterSpacing,
    },
  };
}

/**
 * cellHeight は 1 行の高さだけを返す。
 *
 * <p>**面が建つ前は、渡された箱で代用する。** 指で流す側はこれを touchmove の
 * たびに呼ぶ。建つのを待って 0 を返すと、最初のひと引きが死ぬ。
 */
export function cellHeight(view: MeasurableTerminal, whileBuilding: HTMLElement): number {
  return perRow((surface(view) ?? whileBuilding).getBoundingClientRect().height, view.rows);
}
