import { apiClient } from "./client";
import { putJSON } from "./guards";
import type { components } from "./schema";
import { validateOpenAPISchema } from "./validators.generated";

export type AppearanceSettings = components["schemas"]["AppearanceSettings"];
type Metadata = components["schemas"]["Metadata"];

export type AppearanceApi = {
  appearanceSettings(): Promise<AppearanceSettings>;
  setAppearanceSettings(settings: AppearanceSettings): Promise<void>;
};

// The accent is kept in the synced metadata so every machine shows the same
// colour; the theme is not here because it stays in this browser.
export const appearanceApi: AppearanceApi = {
  async appearanceSettings() {
    const metadata = validateOpenAPISchema<Metadata>("Metadata", await apiClient.read("/api/v1/metadata"));
    return metadata.appearance ?? {};
  },
  async setAppearanceSettings(settings) {
    await putJSON("/api/v1/metadata/appearance", settings);
  },
};
