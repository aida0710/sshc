// 端末の字体。
//
// **同梱するのは、選ぶものが端末に無いからである。** 手元の等幅を名前で並べても、
// Android にはその実物が入っていない——名前は何にも解決せず、選んでも何も
// 変わらない選択肢になる。だから 1 つは自分で運ぶ。
//
// **`stack` は CSS の font-family そのものである。** xterm は文字列をそのまま
// 受け取り、桁の幅をそこから実測する。同梱の名前だけを書いて代替を書かないと、
// 読み込みが失敗した瞬間に端末が読めなくなる。
//
// 名前と実物の対応は fonts.test.ts が index.css と public/fonts を読んで
// 突き合わせる——**選べるのに入っていない字体**も、**入っているのに選べない
// 字体**も、動かしてみるまで気付かない種類の壊れ方である。

export type Font = { readonly name: string; readonly label: string; readonly stack: string };

/**
 * defaultStack は、何も選ばれていないときの字体である。
 *
 * <p>Android には SF Mono も Menlo も無い。**ui-monospace はそこで何にも解決
 * しないことがある**ので、その端末が実際に持っている等幅を並べる。
 */
export const defaultStack =
  'ui-monospace, SFMono-Regular, "SF Mono", Menlo, "Roboto Mono", "Droid Sans Mono", monospace';

export const fonts: readonly Font[] = [
  {
    name: "jetbrains-mono",
    label: "JetBrains Mono",
    stack: `"JetBrains Mono", ${defaultStack}`,
  },
];

/**
 * knownFont は、その名前に字体があれば返す。
 *
 * <p>**知らない名前は null である。** 字体を 1 つ改名した日に、それを選んで
 * いた人の端末が開けなくなってはならない。
 */
export function knownFont(name: string): Font | null {
  return fonts.find((font) => font.name === name) ?? null;
}

/** fontStack は、選ばれた名前を xterm に渡せる綴りへ変える。 */
export function fontStack(name: string): string {
  return knownFont(name)?.stack ?? defaultStack;
}
