import type { ITheme } from "@xterm/xterm";

// ANSI の 16 色は index.css に `--ui-term-*` として、テーマごとに一度だけ
// 定義されている。xterm.js は色を JS のオプションとして受け取るので、ここで
// 読んで渡す。生の hex をコンポーネントに書かないので、palette.test.ts に
// 例外（palette-exempt）を足さずに済む。
const tokens = [
  "bg", "fg", "cursor", "selection",
  "black", "red", "green", "yellow", "blue", "magenta", "cyan", "white",
  "bright-black", "bright-red", "bright-green", "bright-yellow",
  "bright-blue", "bright-magenta", "bright-cyan", "bright-white",
] as const;

type Token = (typeof tokens)[number];

function read(styles: CSSStyleDeclaration, token: Token): string {
  return styles.getPropertyValue(`--ui-term-${token}`).trim();
}

// terminalTheme は、いま適用されているテーマの端末配色を返す。
//
// 引数の element は、トークンが解決されるスコープである。ルートに `data-theme`
// が立つので既定はそこだが、テストが差し替えられるように引数にしてある。
export function terminalTheme(element: Element = document.documentElement): ITheme {
  const styles = getComputedStyle(element);
  return {
    background: read(styles, "bg"),
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
