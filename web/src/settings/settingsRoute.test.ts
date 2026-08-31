import { describe, expect, it } from "vitest";
import {
  canonicalSettingsPath,
  parseSettingsPage,
  settingsPagePath,
  settingsPages,
} from "./settingsRoute";

describe("settings routes", () => {
  it.each(settingsPages)("maps %s to a dedicated page", (page) => {
    const path = settingsPagePath(page);
    expect(parseSettingsPage(path)).toBe(page);
    expect(parseSettingsPage(`${path}/`)).toBe(page);
    expect(canonicalSettingsPath(`${path}/`)).toBe(path);
  });

  it("keeps the legacy settings URL on the engine page", () => {
    expect(parseSettingsPage("/settings")).toBe("Engine");
    expect(canonicalSettingsPath("/settings")).toBe("/settings");
  });

  it("rejects an unknown settings page", () => {
    expect(parseSettingsPage("/settings/unknown")).toBeNull();
    expect(canonicalSettingsPath("/settings/unknown")).toBeNull();
  });
});
