import { localApiBase } from "../api/remoteEngines";
import { apiClient } from "../api/client";
import { sendJSON, postJSON } from "../api/guards";
import type { components } from "../api/schema";
import { validateOpenAPISchema } from "../api/validators.generated";
import type { RemoteEntry } from "./api";
import { vpnProblemCodes } from "../vpn/vpnRefusals";

export type FilesystemSpace = components["schemas"]["SFTPFilesystemSpace"];
export type CreateSymlinkRequest = components["schemas"]["SFTPCreateSymlinkRequest"];
export type ChangeSymlinkRequest = components["schemas"]["SFTPChangeSymlinkRequest"];
export type OwnershipRequest = components["schemas"]["SFTPOwnershipRequest"];

const metadataProblems = [
  "sftp_failed", "sftp_conflict", "sftp_exists", "sftp_not_found", "sftp_wrong_type",
  "sftp_permission_denied", "sftp_unsupported_operation", "sftp_ownership_unavailable",
  "sftp_invalid_space", "sftp_metadata_unavailable", "action_token_invalid", "action_token_expired", ...vpnProblemCodes,
];

function symlinkActionTarget(alias: string, request: CreateSymlinkRequest): string {
  const encoded = btoa(String.fromCharCode(...new TextEncoder().encode(request.target)))
    .replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/, "");
  return `${alias}:${request.path}:${encoded}`;
}

async function metadataConfirmation(base: string, kind: string, target: string): Promise<string> {
  const response = await postJSON<unknown>(`${base}/actions`, { kind, target }, undefined, metadataProblems);
  return validateOpenAPISchema<components["schemas"]["IssueActionResponse"]>("IssueActionResponse", response).token;
}

// createRemoteMetadataApi is this API of the engine at base (engineApiBase): this machine's, or a run target's as this
// machine's engine forwards it.
export function createRemoteMetadataApi(base: string) {
  return {
    async createSymlink(alias: string, request: CreateSymlinkRequest): Promise<RemoteEntry> {
      const token = await metadataConfirmation(base, "sftp.symlink.create", symlinkActionTarget(alias, request));
      return validateOpenAPISchema<RemoteEntry>("SFTPEntry", await postJSON<unknown>(`${base}/sftp/${encodeURIComponent(alias)}/symlink`, request, token, metadataProblems));
    },
    async changeSymlink(alias: string, request: ChangeSymlinkRequest): Promise<RemoteEntry> {
      const token = await metadataConfirmation(base, "sftp.symlink", symlinkActionTarget(alias, request));
      return validateOpenAPISchema<RemoteEntry>("SFTPEntry", await sendJSON<unknown>(`${base}/sftp/${encodeURIComponent(alias)}/symlink`, { method: "PATCH", body: request, actionToken: token, locallyHandledCodes: metadataProblems }));
    },
    async changeOwnership(alias: string, request: OwnershipRequest): Promise<RemoteEntry> {
      const token = await metadataConfirmation(base, "sftp.ownership", `${alias}:${request.path}:${request.uid}:${request.gid}`);
      return validateOpenAPISchema<RemoteEntry>("SFTPEntry", await sendJSON<unknown>(`${base}/sftp/${encodeURIComponent(alias)}/ownership`, { method: "PATCH", body: request, actionToken: token, locallyHandledCodes: metadataProblems }));
    },
    async filesystemSpace(alias: string, path: string): Promise<FilesystemSpace> {
      const response = await apiClient.read(`${base}/sftp/${encodeURIComponent(alias)}/space?path=${encodeURIComponent(path)}`, { locallyHandledCodes: metadataProblems });
      return validateOpenAPISchema<FilesystemSpace>("SFTPFilesystemSpace", response);
    },
  };
}

export type RemoteMetadataApi = ReturnType<typeof createRemoteMetadataApi>;

export const remoteMetadataApi: RemoteMetadataApi = createRemoteMetadataApi(localApiBase);
