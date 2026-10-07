import { sendJSON } from "./guards";
import { apiClient } from "./client";
import type { components } from "./schema";
import { validateOpenAPISchema } from "./validators.generated";

export type UpdateStatus = components["schemas"]["UpdateStatus"];
export type UpdateJob = components["schemas"]["UpdateJob"];
export type UpdatePreview = components["schemas"]["UpdatePreview"];

export type UpdateApi = {
  updateStatus(): Promise<UpdateStatus>;
  previewUpdate(target: string): Promise<UpdatePreview>;
  startUpdate(target: string, actionToken: string): Promise<UpdateJob>;
};

export const updateApi: UpdateApi = {
  async updateStatus() {
    const status = await apiClient.read("/api/v1/update", {
      locallyHandledCodes: ["network_request_failed", "update_state_failed"],
    });
    return validateOpenAPISchema<UpdateStatus>("UpdateStatus", status);
  },
  previewUpdate(target) {
    return sendJSON<UpdatePreview>("/api/v1/update/preview", {
      method: "POST",
      body: { target },
      locallyHandledCodes: ["update_check_failed", "update_state_failed"],
    });
  },
  startUpdate(target, actionToken) {
    return sendJSON<UpdateJob>("/api/v1/update", {
      method: "POST",
      body: { target },
      actionToken,
      locallyHandledCodes: ["update_check_failed", "update_state_failed"],
    });
  },
};
