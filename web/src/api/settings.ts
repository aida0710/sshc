import { apiClient } from "./client";
import { terminalSessionsApi } from "./terminalSessions";
import { patchJSON, putJSON } from "./guards";
import type { components } from "./schema";
import { validateOpenAPISchema } from "./validators.generated";

export type { TerminalSettings } from "../settings/terminalSettings";
export type LocalShellProfile = components["schemas"]["LocalShellProfile"];
export type LocalShellProfileList = components["schemas"]["LocalShellProfileList"];
export type TerminalAppearance = components["schemas"]["TerminalAppearance"];
export type TerminalBackground = components["schemas"]["TerminalBackground"];
export type TerminalBackgroundList = components["schemas"]["TerminalBackgroundList"];

// The settings themselves (the global settings, the presets and the machines) are read and written through
// settings/settingsStore. This is what the settings pages ask the engine besides them: this machine's shells
// and the background images.
export type SettingsApi = {
  localShellProfiles?(): Promise<LocalShellProfileList>;
  terminalBackgrounds(): Promise<TerminalBackgroundList>;
  addTerminalBackground(
    suggested: string,
    image: Blob,
  ): Promise<TerminalBackground>;
  setTerminalBackgroundCapacity(capacityMiB: number): Promise<TerminalBackgroundList>;
  renameTerminalBackground(name: string, nextName: string): Promise<TerminalBackground>;
  deleteTerminalBackground(name: string): Promise<void>;
};

function validateBackground(value: unknown): TerminalBackground {
  return validateOpenAPISchema<TerminalBackground>("TerminalBackground", value);
}

export const settingsApi: SettingsApi = {
  // Settings choose among this machine's shells, whichever engine runs the
  // terminals.
  localShellProfiles: () => terminalSessionsApi.shellProfiles(),
  async terminalBackgrounds() {
    return validateOpenAPISchema<TerminalBackgroundList>(
      "TerminalBackgroundList",
      await apiClient.read("/api/v1/terminal/backgrounds"),
    );
  },
  async addTerminalBackground(suggested, image) {
    return validateBackground(
      await apiClient.mutate<unknown>(
        `/api/v1/terminal/backgrounds?name=${encodeURIComponent(suggested)}`,
        {
          method: "POST",
          headers: { "Content-Type": "application/octet-stream" },
          body: image,
        },
      ),
    );
  },
  async setTerminalBackgroundCapacity(capacityMiB) {
    return validateOpenAPISchema<TerminalBackgroundList>(
      "TerminalBackgroundList",
      await putJSON<unknown>("/api/v1/terminal/backgrounds/capacity", { capacityMiB }),
    );
  },
  async renameTerminalBackground(name, nextName) {
    return validateBackground(
      await patchJSON<unknown>(`/api/v1/terminal/backgrounds/${encodeURIComponent(name)}`, { name: nextName }),
    );
  },
  async deleteTerminalBackground(name) {
    await apiClient.mutate<unknown>(
      `/api/v1/terminal/backgrounds/${encodeURIComponent(name)}`,
      { method: "DELETE" },
    );
  },
};
