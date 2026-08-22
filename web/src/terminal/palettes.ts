// 端末の配色。
//
// **色そのものはここに無い。** 20 個のトークンは index.css の
// `[data-term-palette="…"]` が持っている。ここにあるのは名前と、画面に出す
// 綴りだけである——生の hex を TypeScript に置くと、ライトとダークで別の値を
// 与える仕組みからも、palette.test.ts からも外れる。
//
// **名前の一覧を 2 か所に書かない。** CSS にしか無い名前は選べない配色になり、
// ここにしか無い名前は選べるのに何も変わらない配色になる。palettes.test.ts が
// CSS を読んで突き合わせる。
//
// 綴りは訳さない。Dracula も Nord も固有名詞である。

export type Palette = { readonly name: string; readonly label: string };

export const palettes: readonly Palette[] = [
  { name: "solarized-dark", label: "Solarized Dark" },
  { name: "solarized-light", label: "Solarized Light" },
  { name: "dracula", label: "Dracula" },
  { name: "nord", label: "Nord" },
  { name: "gruvbox-dark", label: "Gruvbox Dark" },
  { name: "one-dark", label: "One Dark" },
];

/**
 * knownPalette は、その名前に配色があれば返す。
 *
 * <p>**知らない名前は null である。** 手で書かれた綴りひとつで端末が開けなく
 * なってはならない——名乗られた配色が無ければ、端末はアプリのテーマに従う。
 * 配色を 1 つ改名した日に、それを選んでいた人が黙って取り残されない。
 */
export function knownPalette(name: string): Palette | null {
  return palettes.find((palette) => palette.name === name) ?? null;
}
