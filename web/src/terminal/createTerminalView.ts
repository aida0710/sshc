import { FitAddon } from "@xterm/addon-fit";
import { SearchAddon } from "@xterm/addon-search";
import { SerializeAddon } from "@xterm/addon-serialize";
import { Terminal, type ITheme } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
import { fitEmptyAlternateScreen } from "./alternateScreenLength";
import { bufferRangeText } from "./bufferLinks";
import { attachInlineImages } from "./inlineImages";
import { attachLigatures, type LigatureDrawing } from "./ligatures";
import { linkTextNamesTarget, modifierOpensLink, osc8Link } from "./links";
import type { ScreenPoint } from "./tappedLink";
import { openTerminalURL, type TerminalLinkSelection } from "./TerminalLinkPopover";
import { applyCurrentUnicodeWidths } from "./unicodeWidths";
import { attachWebglWhileShown } from "./webgl";

// The width xterm.js gives its scrollbar when none is named.
const scrollbarWidthPx = 14;

export type TerminalViewOptions = {
  // backgroundImage lets the picture behind the terminal show through its cells.
  backgroundImage: boolean;
  cursorBlink: boolean;
  fontFamily: string;
  fontSize: number;
  theme: ITheme;
  scrollback: number;
  webgl: boolean;
  // webglSettled is called when the WebGL renderer has loaded, or could not be.
  webglSettled: () => void;
  ligatures: boolean;
  // selectLink opens what an OSC 8 link offers, for a click without the modifier that opens it at once.
  selectLink: (link: TerminalLinkSelection["link"], point: ScreenPoint) => void;
};

// TerminalViewParts is a terminal opened in its container, with the addons the view still reaches for.
export type TerminalViewParts = {
  view: Terminal;
  fit: FitAddon;
  searchAddon: SearchAddon;
  serializeAddon: SerializeAddon;
  ligatureDrawing: LigatureDrawing;
  renderer: ReturnType<typeof attachWebglWhileShown>;
  dispose: () => void;
};

// createTerminalView builds the xterm terminal for one session and opens it in container: its options, the
// addons for fitting, searching and serializing, inline images, ligatures, the WebGL renderer, and the fix for
// xterm.js's alternate screen after a resize (alternateScreenLength.ts).
export function createTerminalView(container: HTMLElement, options: TerminalViewOptions): TerminalViewParts {
  const view: Terminal = new Terminal({
    allowProposedApi: true,
    allowTransparency: options.backgroundImage,
    cols: 80,
    rows: 24,
    convertEol: false,
    cursorBlink: options.cursorBlink,
    fontFamily: options.fontFamily,
    fontSize: options.fontSize,
    theme: options.theme,
    scrollback: options.scrollback,
    // Clearing the screen at a prompt (Ctrl+L in bash and zsh) moves what
    // was on it into the scrollback instead of erasing it, as VTE, Windows
    // Terminal and Ghostty do. `clear` still empties the scrollback (CSI 3 J).
    scrollOnEraseInDisplay: true,
    // A drag goes to a program that reads the mouse (Claude Code, vim, tmux).
    // xterm.js lets Shift take the drag back for selecting, except on macOS,
    // where only Option does and only with this on.
    macOptionClickForcesSelection: true,
    // Naming the scrollbar's width, xterm.js's own default, turns on the overview ruler in it: the marks of
    // search matches (useTerminalSearch.ts) and of commands (commandScrollbarMarks.ts) are drawn there.
    scrollbar: { width: scrollbarWidthPx },
    // Programs ask whether the kitty keyboard protocol is there (CSI ? u)
    // before they turn it on; xterm.js answers and encodes the keys itself.
    vtExtensions: { kittyKeyboard: true },
    linkHandler: {
      activate: (event, target, range) => {
        const link = osc8Link(target, bufferRangeText(view.buffer.active, range), range.start.x - 1, range.end.x);
        if (link === null) return;
        if (modifierOpensLink(event) && linkTextNamesTarget(link)) {
          openTerminalURL(link.target);
          return;
        }
        options.selectLink(link, event);
      },
    },
  });
  applyCurrentUnicodeWidths(view);
  const alternateScreen = fitEmptyAlternateScreen(view);
  const fit = new FitAddon();
  const searchAddon = new SearchAddon({ highlightLimit: 1000 });
  view.loadAddon(fit);
  view.loadAddon(searchAddon);
  const serializeAddon = new SerializeAddon();
  view.loadAddon(serializeAddon);
  view.open(container);
  const inlineImages = attachInlineImages(view);
  const ligatureDrawing = attachLigatures(view, options.ligatures);
  const renderer = attachWebglWhileShown(view, {
    enabled: options.webgl,
    backgroundImage: options.backgroundImage,
    webglSettled: options.webglSettled,
  });
  return {
    view,
    fit,
    searchAddon,
    serializeAddon,
    ligatureDrawing,
    renderer,
    dispose: () => {
      alternateScreen.dispose();
      inlineImages.dispose();
      ligatureDrawing.dispose();
      renderer.dispose();
      view.dispose();
    },
  };
}
