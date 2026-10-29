import type { TerminalSettings } from "../api/settings";
import { appearanceOf } from "../terminal/appearance";

// The font sizes the engine accepts (EmbeddedTerminal.fontSize).
export const fontSizeRange = { min: 8, max: 32 } as const;

// Every field is kept as the text the user typed; numbers are read once, on
// save, so that a half-typed value never snaps back under the cursor.
export type TerminalDraft = {
  startDirectory: string;
  localShellProfile: string;
  maxSessions: string;
  scrollback: string;
  browserScrollbackLines: string;
  retainedTerminals: string;
  fontSize: string;
  verbosity: string;
  reconnect: string;
  palette: string;
  font: string;
  background: string;
  tint: number | undefined;
  copyOnSelect: boolean;
  rightClickPaste: boolean;
  webgl: boolean;
  osc52: boolean;
  jisYenBackslash: boolean;
  composerEnterSends: boolean;
  ligatures: boolean;
  statusLine: boolean;
  commandMarks: boolean;
  stickyCommand: boolean;
  commandScrollbarMarks: boolean;
};

export const initialTerminalDraft: TerminalDraft = {
  startDirectory: "", localShellProfile: "", maxSessions: "", scrollback: "", browserScrollbackLines: "", retainedTerminals: "", fontSize: "",
  verbosity: "0", reconnect: "", palette: "", font: "", background: "", tint: undefined,
  copyOnSelect: true, rightClickPaste: true, webgl: true, osc52: false, jisYenBackslash: false, composerEnterSends: false, ligatures: false, statusLine: true,
  commandMarks: true, stickyCommand: true, commandScrollbarMarks: true,
};

export function draftOf(settings: TerminalSettings): TerminalDraft {
  const text = (value: number | undefined) => value === undefined ? "" : String(value);
  return {
    startDirectory: settings.startDirectory ?? "",
    localShellProfile: settings.localShellProfile ?? "",
    maxSessions: text(settings.maxSessions),
    scrollback: text(settings.scrollbackBytes),
    browserScrollbackLines: text(settings.browserScrollbackLines),
    retainedTerminals: text(settings.retainedTerminals),
    fontSize: text(settings.fontSize),
    verbosity: String(settings.verbosity ?? 0),
    reconnect: text(settings.reconnect),
    palette: settings.appearance?.palette ?? "",
    font: settings.appearance?.font ?? "",
    background: settings.appearance?.background ?? "",
    tint: settings.appearance?.backgroundTint,
    copyOnSelect: settings.copyOnSelect ?? true,
    rightClickPaste: settings.rightClickPaste ?? true,
    webgl: settings.webgl ?? true,
    osc52: settings.osc52 ?? false,
    jisYenBackslash: settings.jisYenBackslash ?? false,
    composerEnterSends: settings.composerEnterSends ?? false,
    ligatures: settings.ligatures ?? false,
    statusLine: settings.statusLine ?? true,
    commandMarks: settings.commandMarks ?? true,
    stickyCommand: settings.stickyCommand ?? true,
    commandScrollbarMarks: settings.commandScrollbarMarks ?? true,
  };
}

function numberOr(text: string): number | undefined {
  const trimmed = text.trim();
  if (trimmed === "") return undefined;
  const value = Number(trimmed);
  return Number.isSafeInteger(value) ? value : Number.NaN;
}

// isFontSizeUnusable says whether the typed font size is one the engine would refuse, so a refused save can
// point at the font size rather than at the limits.
export function isFontSizeUnusable(draft: TerminalDraft): boolean {
  const size = numberOr(draft.fontSize);
  return size !== undefined && (Number.isNaN(size) || size < fontSizeRange.min || size > fontSizeRange.max);
}

// Turns the draft back into the settings document, leaving out every field
// that still has its default so the stored document says only what changed.
export function settingsOf(draft: TerminalDraft): TerminalSettings | null {
  const sessions = numberOr(draft.maxSessions);
  const bytes = numberOr(draft.scrollback);
  const lines = numberOr(draft.browserScrollbackLines);
  const retained = numberOr(draft.retainedTerminals);
  const size = numberOr(draft.fontSize);
  if ([sessions, bytes, lines, retained, size].some(Number.isNaN)) return null;
  const directory = draft.startDirectory.trim();
  return {
    ...(directory === "" ? {} : { startDirectory: directory }),
    ...(sessions === undefined ? {} : { maxSessions: sessions }),
    ...(bytes === undefined ? {} : { scrollbackBytes: bytes }),
    ...(lines === undefined ? {} : { browserScrollbackLines: lines }),
    ...(retained === undefined ? {} : { retainedTerminals: retained }),
    ...(size === undefined ? {} : { fontSize: size }),
    ...(draft.verbosity === "0" ? {} : { verbosity: Number(draft.verbosity) }),
    ...(draft.reconnect === "" ? {} : { reconnect: Number(draft.reconnect) }),
    ...(draft.copyOnSelect ? {} : { copyOnSelect: false }),
    ...(draft.rightClickPaste ? {} : { rightClickPaste: false }),
    ...(draft.webgl ? {} : { webgl: false }),
    ...(draft.osc52 ? { osc52: true } : {}),
    ...(draft.jisYenBackslash ? { jisYenBackslash: true } : {}),
    ...(draft.composerEnterSends ? { composerEnterSends: true } : {}),
    ...(draft.ligatures ? { ligatures: true } : {}),
    ...(draft.statusLine ? {} : { statusLine: false }),
    ...(draft.commandMarks ? {} : { commandMarks: false }),
    ...(draft.stickyCommand ? {} : { stickyCommand: false }),
    ...(draft.commandScrollbarMarks ? {} : { commandScrollbarMarks: false }),
    ...(draft.localShellProfile === "" ? {} : { localShellProfile: draft.localShellProfile }),
    ...appearanceOf({ palette: draft.palette, font: draft.font, background: draft.background, tint: draft.tint }),
  };
}
