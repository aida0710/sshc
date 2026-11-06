import type { MessageKey } from "../i18n/messages";
import { appearanceOf } from "../terminal/appearance";
import { maxBrowserScrollbackLines, minBrowserScrollbackLines } from "../terminal/browserScrollback";
import { fontSizeRange } from "../terminal/fontSize";
import { maxRetainedTerminals, minRetainedTerminals } from "../terminal/splitView/retainedSessions";
import type { PresetTerminalSettings } from "./terminalSettings";

const bytesPerKiB = 1024;

// The numbers the engine accepts (terminal.MinScrollback and the limits in internal/application/metadata.go).
// The replay buffer is typed in KiB and stored in bytes.
export const maxSessionsRange = { min: 1, max: 200 } as const;
export const replayBufferKiBRange = { min: 16, max: 4096 } as const;
export const defaultMaxSessions = 50;
export const defaultReplayBufferKiB = 256;

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
  palette: string;
  font: string;
  background: string;
  tint: number | undefined;
  copyOnSelect: boolean;
  rightClickPaste: boolean;
  webgl: boolean;
  jisYenBackslash: boolean;
  composerEnterSends: boolean;
  ligatures: boolean;
  statusLine: boolean;
  commandMarks: boolean;
  stickyCommand: boolean;
  commandScrollbarMarks: boolean;
  runningCommandTitle: boolean;
};

export const initialTerminalDraft: TerminalDraft = {
  startDirectory: "", localShellProfile: "", maxSessions: "", scrollback: "", browserScrollbackLines: "", retainedTerminals: "", fontSize: "",
  palette: "", font: "", background: "", tint: undefined,
  copyOnSelect: true, rightClickPaste: true, webgl: true, jisYenBackslash: false, composerEnterSends: false, ligatures: false, statusLine: true,
  commandMarks: true, stickyCommand: true, commandScrollbarMarks: true, runningCommandTitle: true,
};

export function draftOf(settings: PresetTerminalSettings): TerminalDraft {
  const text = (value: number | undefined) => value === undefined ? "" : String(value);
  return {
    startDirectory: settings.startDirectory ?? "",
    localShellProfile: settings.localShellProfile ?? "",
    maxSessions: text(settings.maxSessions),
    // A value written by hand may not be whole KiB; it reads as the nearest, and is saved so with the next change.
    scrollback: text(settings.scrollbackBytes === undefined ? undefined : Math.round(settings.scrollbackBytes / bytesPerKiB)),
    browserScrollbackLines: text(settings.browserScrollbackLines),
    retainedTerminals: text(settings.retainedTerminals),
    fontSize: text(settings.fontSize),
    palette: settings.appearance?.palette ?? "",
    font: settings.appearance?.font ?? "",
    background: settings.appearance?.background ?? "",
    tint: settings.appearance?.backgroundTint,
    copyOnSelect: settings.copyOnSelect ?? true,
    rightClickPaste: settings.rightClickPaste ?? true,
    webgl: settings.webgl ?? true,
    jisYenBackslash: settings.jisYenBackslash ?? false,
    composerEnterSends: settings.composerEnterSends ?? false,
    ligatures: settings.ligatures ?? false,
    statusLine: settings.statusLine ?? true,
    commandMarks: settings.commandMarks ?? true,
    stickyCommand: settings.stickyCommand ?? true,
    commandScrollbarMarks: settings.commandScrollbarMarks ?? true,
    runningCommandTitle: settings.runningCommandTitle ?? true,
  };
}

function numberOr(text: string): number | undefined {
  const trimmed = text.trim();
  if (trimmed === "") return undefined;
  const value = Number(trimmed);
  return Number.isSafeInteger(value) ? value : Number.NaN;
}

export type TypedNumber = {
  field: "maxSessions" | "scrollback" | "browserScrollbackLines" | "retainedTerminals" | "fontSize";
  range: { readonly min: number; readonly max: number };
  outOfRange: MessageKey;
};

const typedNumbers: TypedNumber[] = [
  { field: "maxSessions", range: maxSessionsRange, outOfRange: "terminal.maxSessionsOutOfRange" },
  { field: "scrollback", range: replayBufferKiBRange, outOfRange: "terminal.replayBufferOutOfRange" },
  { field: "browserScrollbackLines", range: { min: minBrowserScrollbackLines, max: maxBrowserScrollbackLines }, outOfRange: "terminal.browserScrollbackOutOfRange" },
  { field: "retainedTerminals", range: { min: minRetainedTerminals, max: maxRetainedTerminals }, outOfRange: "terminal.retainedTerminalsOutOfRange" },
  { field: "fontSize", range: fontSizeRange, outOfRange: "terminal.fontSizeOutOfRange" },
];

// firstUnusableNumber is the first typed number the engine would refuse, so the page can say which and what it
// accepts before saving. A blank field is the default, and always usable.
export function firstUnusableNumber(draft: TerminalDraft): TypedNumber | null {
  return typedNumbers.find(({ field, range }) => {
    const value = numberOr(draft[field]);
    return value !== undefined && (Number.isNaN(value) || value < range.min || value > range.max);
  }) ?? null;
}

// Turns the draft back into the preset's terminal settings, leaving out every field
// that still has its default so the stored document says only what changed. The caller has
// checked the numbers with firstUnusableNumber.
export function settingsOf(draft: TerminalDraft): PresetTerminalSettings {
  const sessions = numberOr(draft.maxSessions);
  const replayBufferKiB = numberOr(draft.scrollback);
  const lines = numberOr(draft.browserScrollbackLines);
  const retained = numberOr(draft.retainedTerminals);
  const size = numberOr(draft.fontSize);
  const directory = draft.startDirectory.trim();
  return {
    ...(directory === "" ? {} : { startDirectory: directory }),
    ...(sessions === undefined ? {} : { maxSessions: sessions }),
    ...(replayBufferKiB === undefined ? {} : { scrollbackBytes: replayBufferKiB * bytesPerKiB }),
    ...(lines === undefined ? {} : { browserScrollbackLines: lines }),
    ...(retained === undefined ? {} : { retainedTerminals: retained }),
    ...(size === undefined ? {} : { fontSize: size }),
    ...(draft.copyOnSelect ? {} : { copyOnSelect: false }),
    ...(draft.rightClickPaste ? {} : { rightClickPaste: false }),
    ...(draft.webgl ? {} : { webgl: false }),
    ...(draft.jisYenBackslash ? { jisYenBackslash: true } : {}),
    ...(draft.composerEnterSends ? { composerEnterSends: true } : {}),
    ...(draft.ligatures ? { ligatures: true } : {}),
    ...(draft.statusLine ? {} : { statusLine: false }),
    ...(draft.commandMarks ? {} : { commandMarks: false }),
    ...(draft.stickyCommand ? {} : { stickyCommand: false }),
    ...(draft.commandScrollbarMarks ? {} : { commandScrollbarMarks: false }),
    ...(draft.runningCommandTitle ? {} : { runningCommandTitle: false }),
    ...(draft.localShellProfile === "" ? {} : { localShellProfile: draft.localShellProfile }),
    ...appearanceOf({ palette: draft.palette, font: draft.font, background: draft.background, tint: draft.tint }),
  };
}
