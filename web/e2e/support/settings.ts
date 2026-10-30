import type { Page } from "@playwright/test";
import type { InterfaceStateKey, SettingsDocument, SettingsPreset } from "../../src/settings/settingsStore";

// The metadata.json schema this build reads and writes (application.MetadataSchemaVersion).
export const metadataSchemaVersion = 10;

// standardPresetMetadata is a metadata.json whose standard preset, the one a machine uses until it chooses
// another, holds preset.
export function standardPresetMetadata(preset: Omit<SettingsPreset, "id">): string {
  return JSON.stringify({ schemaVersion: metadataSchemaVersion, presets: [{ id: "default", ...preset }] });
}

// The engine keeps the settings and this machine's screen state (GET /api/v1/settings). It writes the state
// to its file only every half minute, so a test reads and seeds it through the API, as the page does.

export async function readSettings(page: Page): Promise<SettingsDocument> {
  return page.evaluate(async () => {
    const csrf = window.sessionStorage.getItem("sshc.session.csrf") ?? "";
    const response = await fetch("/api/v1/settings", { credentials: "same-origin", headers: { "X-SSHC-CSRF": csrf } });
    if (!response.ok) throw new Error(`reading the settings failed with ${response.status}`);
    return response.json() as Promise<SettingsDocument>;
  });
}

// readMachineState is the screen state this machine keeps under key, or "" when none is kept.
export async function readMachineState(page: Page, key: InterfaceStateKey): Promise<string> {
  return (await readSettings(page)).state.interface?.[key] ?? "";
}

// keepMachineState puts screen state in place for a page loaded after it; a page already open reads it on its
// next poll of the settings.
export async function keepMachineState(page: Page, key: InterfaceStateKey, value: string): Promise<void> {
  await page.evaluate(async ({ key, value }) => {
    const csrf = window.sessionStorage.getItem("sshc.session.csrf") ?? "";
    const response = await fetch("/api/v1/settings/state", {
      method: "PUT",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json", "X-SSHC-CSRF": csrf },
      body: JSON.stringify({ interface: { [key]: value } }),
    });
    if (!response.ok) throw new Error(`keeping the screen state failed with ${response.status}`);
  }, { key, value });
}
