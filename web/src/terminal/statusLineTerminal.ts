import { Terminal, type ITerminalOptions } from "@xterm/xterm";
import { applyCurrentUnicodeWidths } from "./unicodeWidths";

// The status line (statusLine.ts) is a terminal of its own, one row as wide as
// the screen, so that what a program writes there keeps its colours and moves
// its cursor as on the screen. It is made the first time a program shows it.

// The options the status line takes from the screen, so that it reads as part of the same terminal.
const sharedOptions = ["fontFamily", "fontSize", "fontWeight", "fontWeightBold", "letterSpacing", "lineHeight"] as const satisfies readonly (keyof ITerminalOptions)[];

// A transparent background leaves the status line's colour to the stylesheet.
const transparent = "#00000000";

export type StatusLineTerminal = {
  write(text: string): void;
  // clear empties the line, in order with what was written before it.
  clear(): void;
  // open draws the line into the element; the element stays for the life of the screen.
  open(element: HTMLElement): void;
  dispose(): void;
};

export function createStatusLineTerminal(screen: Terminal): StatusLineTerminal {
  let line: Terminal | null = null;
  let followedTheme: ITerminalOptions["theme"] | null = null;
  const follow = () => {
    if (line === null) return;
    for (const name of sharedOptions) {
      if (line.options[name] !== screen.options[name]) line.options[name] = screen.options[name] as never;
    }
    if (followedTheme !== screen.options.theme) {
      followedTheme = screen.options.theme;
      line.options.theme = { ...followedTheme, background: transparent };
    }
    if (line.cols !== screen.cols) line.resize(screen.cols, 1);
  };
  const terminal = () => {
    if (line === null) {
      line = new Terminal({ cols: screen.cols, rows: 1, scrollback: 0, allowProposedApi: true, allowTransparency: true, disableStdin: true, cursorInactiveStyle: "none" });
      applyCurrentUnicodeWidths(line);
      follow();
    }
    return line;
  };
  // The screen renders after any change to its options and after a resize.
  const subscriptions = [screen.onRender(follow), screen.onResize(follow)];
  return {
    write: (text) => terminal().write(text),
    clear: () => terminal().write("\u001bc"),
    open(element) {
      const current = terminal();
      if (current.element !== undefined) return;
      current.open(element);
      // The line takes no input: Tab passes it by, and a click there leaves the typing on the screen.
      current.textarea?.setAttribute("tabindex", "-1");
      element.addEventListener("mousedown", (event) => {
        event.preventDefault();
        event.stopPropagation();
        screen.focus();
      }, { capture: true });
    },
    dispose() {
      for (const subscription of subscriptions) subscription.dispose();
      line?.dispose();
      line = null;
    },
  };
}
