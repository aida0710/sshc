import { describe, expect, it } from "vitest";
import { hostSettingsToDrawWith, type HostSettings } from "./hostSettings";

const none: HostSettings = { hostAppearance: new Map(), hostVPN: new Map() };
const withFont: HostSettings = {
  hostAppearance: new Map([["web", { palette: "", font: "jetbrains-mono", background: "" }]]),
  hostVPN: new Map([["web", "office"]]),
};

describe("hostSettingsToDrawWith", () => {
  it("keeps what the terminals were drawn with while the engine's settings are not read yet", () => {
    expect(hostSettingsToDrawWith({ current: none, read: false, drawnWith: withFont })).toBe(withFont);
  });

  it("takes the engine's settings once they are read, even when no connection asks for anything", () => {
    expect(hostSettingsToDrawWith({ current: none, read: true, drawnWith: withFont })).toBe(none);
  });

  it("draws an engine shown for the first time with what the screens hold", () => {
    expect(hostSettingsToDrawWith({ current: withFont, read: false, drawnWith: undefined })).toBe(withFont);
  });
});
