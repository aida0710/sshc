import { localApiBase } from "../api/remoteEngines";
import { sendJSON } from "../api/guards";
import type { components } from "../api/schema";
import { validateOpenAPISchema } from "../api/validators.generated";
import type { RemoteEntry } from "./api";

// Expected refusals belong in the pane or its confirmation dialog.
const localMutationProblems = [
  "invalid_request", "sftp_failed", "sftp_permission_denied", "sftp_not_found",
  "sftp_conflict", "sftp_exists", "sftp_unsupported_entry", "sftp_traversal_limit",
  "action_token_required", "action_token_invalid", "action_token_expired", "too_many_confirmations",
];

// createLocalMutationApi is this API of the engine at base (engineApiBase): this machine's, or a run target's as this
// machine's engine forwards it.
export function createLocalMutationApi(base: string) {
  return {
    async mkdir(directory: string, name: string): Promise<RemoteEntry> {
      return validateOpenAPISchema<RemoteEntry>("SFTPEntry", await sendJSON(`${base}/sftp/local/directories`, {
        method: "POST", body: { directory, name }, locallyHandledCodes: localMutationProblems,
      }));
    },
    async rename(entry: RemoteEntry, name: string): Promise<RemoteEntry> {
      return validateOpenAPISchema<RemoteEntry>("SFTPEntry", await sendJSON(`${base}/sftp/local/rename`, {
        method: "POST", body: { path: entry.path, name, expectedRevision: entry.revision }, locallyHandledCodes: localMutationProblems,
      }));
    },
    async remove(entries: RemoteEntry[]): Promise<void> {
      const selection = { entries: entries.map((entry) => ({ path: entry.path, expectedRevision: entry.revision })) };
      const plan = validateOpenAPISchema<components["schemas"]["SFTPLocalDeletePlan"]>("SFTPLocalDeletePlan", await sendJSON(`${base}/sftp/local/delete-plan`, {
        method: "POST", body: selection, locallyHandledCodes: localMutationProblems,
      }));
      await sendJSON(`${base}/sftp/local/delete`, {
        method: "POST", body: { ...selection, expectedRevision: plan.revision },
        actionToken: plan.actionToken, locallyHandledCodes: localMutationProblems,
      });
    },
  };
}

export type LocalMutationApi = ReturnType<typeof createLocalMutationApi>;

export const localMutationApi: LocalMutationApi = createLocalMutationApi(localApiBase);
