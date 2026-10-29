import type { components } from "../api/schema";
import type { TerminalSettings } from "../api/settings";

// A named set of how terminals look and behave in a browser (#467), synced so
// that each browser can choose the one it uses. The standard set is the
// terminal settings themselves and has no entry of its own.
export type TerminalPreset = components["schemas"]["TerminalPreset"];

// standardPresetId names the standard set: the terminal settings as they are.
export const standardPresetId = "default";

// The settings a preset holds: what the browser applies. The rest (session
// limits, the replay buffer, reconnects, the connection log, where a local shell
// starts and which shell, and OSC 52) the engine applies or every terminal
// shares, so it is one value for every browser and the engine refuses it in a
// preset.
const presetKeys = [
  "fontSize",
  "browserScrollbackLines",
  "retainedTerminals",
  "copyOnSelect",
  "rightClickPaste",
  "webgl",
  "jisYenBackslash",
  "composerEnterSends",
  "ligatures",
  "statusLine",
  "commandMarks",
  "stickyCommand",
  "commandScrollbarMarks",
  "appearance",
] as const satisfies readonly (keyof TerminalSettings)[];

type PresetKey = (typeof presetKeys)[number];
export type TerminalPresetSettings = Pick<TerminalSettings, PresetKey>;

function pick(settings: TerminalSettings, keep: (key: keyof TerminalSettings) => boolean): TerminalSettings {
  return Object.fromEntries(Object.entries(settings).filter(([key]) => keep(key as keyof TerminalSettings)));
}

function isPresetKey(key: keyof TerminalSettings): key is PresetKey {
  return (presetKeys as readonly string[]).includes(key);
}

// presetSettingsOf keeps the part of settings a preset holds.
export function presetSettingsOf(settings: TerminalSettings): TerminalPresetSettings {
  return pick(settings, isPresetKey);
}

// sharedSettingsOf keeps the part of settings every browser shares.
export function sharedSettingsOf(settings: TerminalSettings): TerminalSettings {
  return pick(settings, (key) => !isPresetKey(key));
}

// withPreset is settings as a browser that uses preset applies them: the shared
// part as it is, and the preset's part from the preset, where a value it leaves
// out is the default rather than the standard set's.
export function withPreset(settings: TerminalSettings, preset: TerminalPreset | undefined): TerminalSettings {
  if (preset === undefined) return settings;
  return { ...sharedSettingsOf(settings), ...presetSettingsOf(preset.settings) };
}
