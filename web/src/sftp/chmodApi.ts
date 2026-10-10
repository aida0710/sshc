import { sendJSON } from "../api/guards";
import type { components } from "../api/schema";
import { validateOpenAPISchema } from "../api/validators.generated";
import { vpnProblemCodes } from "../vpn/vpnRefusals";

export type ChmodOptions = components["schemas"]["SFTPChmodOptions"];
export type ChmodSelection = components["schemas"]["SFTPChmodSelection"];
export type ChmodPlan = components["schemas"]["SFTPChmodPlan"];
export type ChmodResult = components["schemas"]["SFTPChmodResult"];

// These Problem responses are emitted before any mutation starts. An unknown
// HTTP failure may instead be a lost result and must not claim nothing changed.
export const chmodRefusalCodes = [
  "invalid_request", "sftp_failed", "sftp_conflict", "sftp_not_found", "sftp_wrong_type",
  "sftp_permission_denied", "sftp_unsupported_operation", "sftp_metadata_unavailable",
  "sftp_unsupported_entry", "sftp_traversal_limit", "action_token_required",
  "action_token_invalid", "action_token_expired", "too_many_confirmations", ...vpnProblemCodes,
  "session_required", "invalid_session", "invalid_csrf", "vault_locked",
];

export const chmodApi = {
  async plan(alias: string, selection: ChmodSelection): Promise<ChmodPlan> {
    return validateOpenAPISchema<ChmodPlan>("SFTPChmodPlan", await sendJSON(`/api/v1/sftp/${encodeURIComponent(alias)}/mode-plan`, {
      method: "POST", body: selection, locallyHandledCodes: chmodRefusalCodes,
    }));
  },
  async apply({ alias, selection, plan }: { alias: string; selection: ChmodSelection; plan: ChmodPlan }): Promise<ChmodResult> {
    return validateOpenAPISchema<ChmodResult>("SFTPChmodResult", await sendJSON(`/api/v1/sftp/${encodeURIComponent(alias)}/modes`, {
      method: "PATCH", body: { ...selection, expectedRevision: plan.revision },
      actionToken: plan.actionToken, locallyHandledCodes: chmodRefusalCodes,
    }));
  },
};
