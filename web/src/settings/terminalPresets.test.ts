import { describe, expect, it } from "vitest";
import { presetSettingsOf, sharedSettingsOf, withPreset, type TerminalPreset } from "./terminalPresets";

describe("withPreset", () => {
  const standard = {
    fontSize: 15, webgl: false, appearance: { palette: "solarized" },
    maxSessions: 20, reconnect: 0, osc52: true, startDirectory: "~/work",
  };

  it("takes what the browser applies from the preset and the rest from the standard set", () => {
    const phone: TerminalPreset = { id: "phone", name: "Phone", settings: { fontSize: 12, composerEnterSends: true } };
    expect(withPreset(standard, phone)).toEqual({
      maxSessions: 20, reconnect: 0, osc52: true, startDirectory: "~/work",
      fontSize: 12, composerEnterSends: true,
    });
  });

  it("leaves the standard set as it is when this browser uses no preset", () => {
    expect(withPreset(standard, undefined)).toBe(standard);
  });
});

describe("presetSettingsOf and sharedSettingsOf", () => {
  it("split the settings with nothing lost and nothing in both", () => {
    const settings = { fontSize: 13, ligatures: true, verbosity: 2, localShellProfile: "zsh", retainedTerminals: 5, scrollbackBytes: 65536 };
    const preset = presetSettingsOf(settings);
    const shared = sharedSettingsOf(settings);
    expect(preset).toEqual({ fontSize: 13, ligatures: true, retainedTerminals: 5 });
    expect(shared).toEqual({ verbosity: 2, localShellProfile: "zsh", scrollbackBytes: 65536 });
  });
});
