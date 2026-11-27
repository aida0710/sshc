import { sendJSON } from "./guards";
import { apiClient } from "./client";
import { localApiBase } from "./remoteEngines";
import type { components } from "./schema";
import { validateOpenAPISchema } from "./validators.generated";

export type UpdateStatus = components["schemas"]["UpdateStatus"];
export type UpdateJob = components["schemas"]["UpdateJob"];
export type UpdatePreview = components["schemas"]["UpdatePreview"];
export type UpdateLog = components["schemas"]["UpdateLog"];

// engineRestartCodes は、更新の終わりにエンジンが起動し直している間の、答えの無い読み取り。読み直すので、共通の
// 知らせを出さない。リモートセッションのエンジンなら、このマシンのエンジンの取り次ぎが、届かない理由を答える。
const engineRestartCodes = [
  "network_request_failed",
  "remote_engine_connect_failed",
  "remote_engine_not_running",
  "remote_engine_unproven",
  "remote_engine_no_entrance",
  "remote_engine_failed",
] as const;

export type UpdateApi = {
  updateStatus(): Promise<UpdateStatus>;
  checkUpdate(): Promise<UpdateStatus>;
  previewUpdate(target: string): Promise<UpdatePreview>;
  startUpdate(target: string, actionToken: string): Promise<UpdateJob>;
  // updateLog reads what the installer and the restart wrote for the current
  // job, from cursor on.
  updateLog(cursor: number): Promise<UpdateLog>;
};

// createUpdateApi は、base（engineApiBase）のエンジンの sshc の更新の API である。リモートセッションを選んでいる間は、
// その実行先の sshc を更新する。
export function createUpdateApi(base: string): UpdateApi {
  return {
    async updateStatus() {
      const status = await apiClient.read(`${base}/update`, {
        locallyHandledCodes: [...engineRestartCodes, "update_state_failed"],
      });
      return validateOpenAPISchema<UpdateStatus>("UpdateStatus", status);
    },
    async checkUpdate() {
      // The endpoint takes no request body; a JSON body would fail the OpenAPI request check.
      const status = await apiClient.mutate<UpdateStatus>(`${base}/update/check`, { method: "POST" }, { locallyHandledCodes: ["update_state_failed", "network_request_failed"] });
      return validateOpenAPISchema<UpdateStatus>("UpdateStatus", status);
    },
    previewUpdate(target) {
      return sendJSON<UpdatePreview>(`${base}/update/preview`, {
        method: "POST",
        body: { target },
        locallyHandledCodes: ["update_check_failed", "update_state_failed"],
      });
    },
    startUpdate(target, actionToken) {
      return sendJSON<UpdateJob>(`${base}/update`, {
        method: "POST",
        body: { target },
        actionToken,
        locallyHandledCodes: ["update_check_failed", "update_state_failed"],
      });
    },
    async updateLog(cursor) {
      // The engine restarts at the end of an update, so a read that gets no
      // answer is expected and retried; the About page explains a failed update.
      const log = await apiClient.read(`${base}/update/log?cursor=${cursor}`, {
        locallyHandledCodes: [...engineRestartCodes, "update_state_failed"],
      });
      return validateOpenAPISchema<UpdateLog>("UpdateLog", log);
    },
  };
}

export const updateApi = createUpdateApi(localApiBase);
