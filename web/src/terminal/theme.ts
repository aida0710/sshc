import type { ITheme } from "@xterm/xterm";

// ANSI の 16 色は index.css に `--ui-term-*` として、テーマごとに一度だけ
// 定義されている。xterm.js は色を JS のオプションとして受け取るので、ここで
// 読んで渡す。生の hex をコンポーネントに書かないので、palette.test.ts に
// 例外（palette-exempt）を足さずに済む。
// **配列ではなく型である。** かつてここは `as const` の配列で、そこから
// `(typeof tokens)[number]` を作っていた。だが配列そのものは一度も読まれない
// ——束に入るだけで、誰も回さない 20 個の文字列だった。
type Token =
  | "bg" | "fg" | "cursor" | "selection"
  | "black" | "red" | "green" | "yellow"
  | "blue" | "magenta" | "cyan" | "white"
  | "bright-black" | "bright-red" | "bright-green" | "bright-yellow"
  | "bright-blue" | "bright-magenta" | "bright-cyan" | "bright-white";

function read(styles: CSSStyleDeclaration, token: Token): string {
  return styles.getPropertyValue(`--ui-term-${token}`).trim();
}

// terminalTheme は、いま適用されているテーマの端末配色を返す。
//
// 引数の element は、トークンが解決されるスコープである。ルートに `data-theme`
// が立つので既定はそこだが、テストが差し替えられるように引数にしてある。
export function terminalTheme(element: Element = document.documentElement, seeThrough = false): ITheme {
  const styles = getComputedStyle(element);
  return {
    // **画像を敷いたら、端末は面を塗らない。** 塗れば画像はその下に隠れる。
    // かぶせる濃さは箱の側が持つので、ここは何も置かないだけでよい。
    background: seeThrough ? "#00000000" : read(styles, "bg"),
    foreground: read(styles, "fg"),
    cursor: read(styles, "cursor"),
    cursorAccent: read(styles, "bg"),
    selectionBackground: read(styles, "selection"),
    black: read(styles, "black"),
    red: read(styles, "red"),
    green: read(styles, "green"),
    yellow: read(styles, "yellow"),
    blue: read(styles, "blue"),
    magenta: read(styles, "magenta"),
    cyan: read(styles, "cyan"),
    white: read(styles, "white"),
    brightBlack: read(styles, "bright-black"),
    brightRed: read(styles, "bright-red"),
    brightGreen: read(styles, "bright-green"),
    brightYellow: read(styles, "bright-yellow"),
    brightBlue: read(styles, "bright-blue"),
    brightMagenta: read(styles, "bright-magenta"),
    brightCyan: read(styles, "bright-cyan"),
    brightWhite: read(styles, "bright-white"),
  };
}
