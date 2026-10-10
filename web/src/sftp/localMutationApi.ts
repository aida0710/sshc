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

export const localMutationApi = {
  async mkdir(directory: string, name: string): Promise<RemoteEntry> {
    return validateOpenAPISchema<RemoteEntry>("SFTPEntry", await sendJSON("/api/v1/sftp/local/directories", {
      method: "POST", body: { directory, name }, locallyHandledCodes: localMutationProblems,
    }));
  },
  async rename(entry: RemoteEntry, name: string): Promise<RemoteEntry> {
    return validateOpenAPISchema<RemoteEntry>("SFTPEntry", await sendJSON("/api/v1/sftp/local/rename", {
      method: "POST", body: { path: entry.path, name, expectedRevision: entry.revision }, locallyHandledCodes: localMutationProblems,
    }));
  },
  async remove(entries: RemoteEntry[]): Promise<void> {
    const selection = { entries: entries.map((entry) => ({ path: entry.path, expectedRevision: entry.revision })) };
    const plan = validateOpenAPISchema<components["schemas"]["SFTPLocalDeletePlan"]>("SFTPLocalDeletePlan", await sendJSON("/api/v1/sftp/local/delete-plan", {
      method: "POST", body: selection, locallyHandledCodes: localMutationProblems,
    }));
    await sendJSON("/api/v1/sftp/local/delete", {
      method: "POST", body: { ...selection, expectedRevision: plan.revision },
      actionToken: plan.actionToken, locallyHandledCodes: localMutationProblems,
    });
  },
};
