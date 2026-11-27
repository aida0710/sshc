import { apiClient } from "./client";
import { localApiBase } from "./remoteEngines";
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
// settings/settingsStore. This is what the settings pages ask the engine besides them: its shells and the
// background images.
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

// createSettingsApi は、base（engineApiBase）のエンジンの、設定の画面が使う API である。shellProfiles は、
// そのエンジンのシェルの一覧（ターミナルの API）。
export function createSettingsApi(base: string, shellProfiles: () => Promise<LocalShellProfileList>): SettingsApi {
  return {
    localShellProfiles: shellProfiles,
    async terminalBackgrounds() {
      return validateOpenAPISchema<TerminalBackgroundList>(
        "TerminalBackgroundList",
        await apiClient.read(`${base}/terminal/backgrounds`),
      );
    },
    async addTerminalBackground(suggested, image) {
      return validateBackground(
        await apiClient.mutate<unknown>(
          `${base}/terminal/backgrounds?name=${encodeURIComponent(suggested)}`,
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
        await putJSON<unknown>(`${base}/terminal/backgrounds/capacity`, { capacityMiB }),
      );
    },
    async renameTerminalBackground(name, nextName) {
      return validateBackground(
        await patchJSON<unknown>(`${base}/terminal/backgrounds/${encodeURIComponent(name)}`, { name: nextName }),
      );
    },
    async deleteTerminalBackground(name) {
      await apiClient.mutate<unknown>(
        `${base}/terminal/backgrounds/${encodeURIComponent(name)}`,
        { method: "DELETE" },
      );
    },
  };
}

export const settingsApi = createSettingsApi(localApiBase, () => terminalSessionsApi.shellProfiles());
