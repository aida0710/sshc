import { apiClient } from "./client";
import type { components } from "./schema";
import { validateOpenAPISchema } from "./validators.generated";

export type UpdateStatus = components["schemas"]["UpdateStatus"];

export type UpdateApi = {
  updateStatus(): Promise<UpdateStatus>;
};

function validateUpdate(value: unknown): UpdateStatus {
  return validateOpenAPISchema<UpdateStatus>("UpdateStatus", value);
}

// Whether a newer release is available for the running engine.
export const updateApi: UpdateApi = {
  async updateStatus() {
    return validateUpdate(await apiClient.read("/api/v1/update"));
  },
};
