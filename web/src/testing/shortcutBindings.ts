import type { Bindings } from "../keyconfig/bindings";
import { refreshPresets } from "../keyconfig/presets";
import { fakeSettingsServer } from "./settingsServer";

// bindShortcutsForTest puts bindings in force: a saved shortcut preset that the preset in use chose.
export async function bindShortcutsForTest(bindings: Bindings): Promise<void> {
  fakeSettingsServer({ presets: [{ id: "default", interface: { shortcutPreset: "test-shortcuts" } }] }, {
    read: async (path) => {
      if (path !== "/api/v1/metadata") throw new Error(`unexpected read ${path}`);
      return { schemaVersion: 10, presets: [{ id: "default" }], shortcutPresets: [{ id: "test-shortcuts", name: "Test", bindings }] };
    },
  });
  await refreshPresets();
}
