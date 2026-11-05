import type { ITheme } from "@xterm/xterm";

type Token =
  | "bg" | "fg" | "cursor" | "selection"
  | "black" | "red" | "green" | "yellow"
  | "blue" | "magenta" | "cyan" | "white"
  | "bright-black" | "bright-red" | "bright-green" | "bright-yellow"
  | "bright-blue" | "bright-magenta" | "bright-cyan" | "bright-white";

function read(styles: CSSStyleDeclaration, token: Token): string {
  return styles.getPropertyValue(`--ui-term-${token}`).trim();
}

export function terminalTheme(element: Element = document.documentElement, seeThrough = false): ITheme {
  const styles = getComputedStyle(element);
  return {
    background: seeThrough ? "#00000000" : read(styles, "bg"),
    foreground: read(styles, "fg"),
    cursor: read(styles, "cursor"),
    cursorAccent: read(styles, "bg"),
    selectionBackground: read(styles, "selection"),
    // xterm draws this border of the scrollbar's marks in white when no colour is given, a bright line down every
    // dark terminal. The marks and the scrollbar stand out without it.
    overviewRulerBorder: "#00000000",
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
