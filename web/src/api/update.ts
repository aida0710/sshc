import { sendJSON } from "./guards";
import { apiClient } from "./client";
import type { components } from "./schema";
import { validateOpenAPISchema } from "./validators.generated";

export type UpdateStatus = components["schemas"]["UpdateStatus"];
export type UpdateJob = components["schemas"]["UpdateJob"];
export type UpdatePreview = components["schemas"]["UpdatePreview"];
export type UpdateLog = components["schemas"]["UpdateLog"];

export type UpdateApi = {
  updateStatus(): Promise<UpdateStatus>;
  checkUpdate(): Promise<UpdateStatus>;
  previewUpdate(target: string): Promise<UpdatePreview>;
  startUpdate(target: string, actionToken: string): Promise<UpdateJob>;
  // updateLog reads what the installer and the restart wrote for the current
  // job, from cursor on.
  updateLog(cursor: number): Promise<UpdateLog>;
};

export const updateApi: UpdateApi = {
  async updateStatus() {
    const status = await apiClient.read("/api/v1/update", {
      locallyHandledCodes: ["network_request_failed", "update_state_failed"],
    });
    return validateOpenAPISchema<UpdateStatus>("UpdateStatus", status);
  },
  async checkUpdate() {
    // The endpoint takes no request body; a JSON body would fail the OpenAPI request check.
    const status = await apiClient.mutate<UpdateStatus>("/api/v1/update/check", { method: "POST" }, { locallyHandledCodes: ["update_state_failed", "network_request_failed"] });
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
  async updateLog(cursor) {
    // The engine restarts at the end of an update, so a read that gets no
    // answer is expected and retried; the About page explains a failed update.
    const log = await apiClient.read(`/api/v1/update/log?cursor=${cursor}`, {
      locallyHandledCodes: ["network_request_failed", "update_state_failed"],
    });
    return validateOpenAPISchema<UpdateLog>("UpdateLog", log);
  },
};
