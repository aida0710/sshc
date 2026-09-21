import { apiClient } from "./client";
import { patchJSON, putJSON } from "./guards";
import type { components } from "./schema";
import { validateOpenAPISchema } from "./validators.generated";

export type Metadata = components["schemas"]["Metadata"];
export type TerminalSettings = components["schemas"]["TerminalSettings"];
export type LocalShellProfile = components["schemas"]["LocalShellProfile"];
export type LocalShellProfileList = components["schemas"]["LocalShellProfileList"];
export type EngineSettings = components["schemas"]["EngineSettings"];
export type TerminalAppearance = components["schemas"]["TerminalAppearance"];
export type TerminalBackground = components["schemas"]["TerminalBackground"];
export type TerminalBackgroundList = components["schemas"]["TerminalBackgroundList"];

export type SettingsApi = {
  terminalSettings(): Promise<TerminalSettings>;
  setTerminalSettings(settings: TerminalSettings): Promise<void>;
  localShellProfiles?(): Promise<LocalShellProfileList>;
  engineSettings(): Promise<EngineSettings>;
  setEngineSettings(settings: EngineSettings): Promise<void>;
  terminalBackgrounds(): Promise<TerminalBackgroundList>;
  addTerminalBackground(
    suggested: string,
    image: Blob,
  ): Promise<TerminalBackground>;
  setTerminalBackgroundCapacity(capacityMiB: number): Promise<TerminalBackgroundList>;
  renameTerminalBackground(name: string, nextName: string): Promise<TerminalBackground>;
  deleteTerminalBackground(name: string): Promise<void>;
};

function validateMetadata(value: unknown): Metadata {
  return validateOpenAPISchema<Metadata>("Metadata", value);
}

function validateLocalShellProfiles(value: unknown): LocalShellProfileList {
  return validateOpenAPISchema<LocalShellProfileList>("LocalShellProfileList", value);
}

function validateBackground(value: unknown): TerminalBackground {
  return validateOpenAPISchema<TerminalBackground>("TerminalBackground", value);
}

// Settings the engine stores for every browser: the embedded terminal, the
// engine itself, the local shell profiles it found, and the background images.
export const settingsApi: SettingsApi = {
  // The engine already clamps stored settings to the contract when it reads
  // them, so the generated validator is the whole check here.
  async terminalSettings() {
    const metadata = validateMetadata(await apiClient.read("/api/v1/metadata"));
    return metadata.embeddedTerminal ?? {};
  },
  async setTerminalSettings(settings) {
    await putJSON("/api/v1/metadata/terminal", settings);
  },
  async localShellProfiles() {
    return validateLocalShellProfiles(
      await apiClient.read("/api/v1/terminal/shell-profiles"),
    );
  },
  async engineSettings() {
    const metadata = validateMetadata(await apiClient.read("/api/v1/metadata"));
    return metadata.engine ?? {};
  },
  async setEngineSettings(settings) {
    await putJSON("/api/v1/metadata/engine", settings);
  },
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
