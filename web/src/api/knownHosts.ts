import { apiClient } from "./client";
import { issueAction, postJSON } from "./guards";
import type { components } from "./schema";
import { validateOpenAPISchema } from "./validators.generated";

export type KnownHostsResponse = components["schemas"]["KnownHostsResponse"];
export type KnownHostEntry = components["schemas"]["KnownHostEntry"];
export type KnownHostsChangeResponse = components["schemas"]["KnownHostsChangeResponse"];
export type KnownHostsScanResponse = components["schemas"]["KnownHostsScanResponse"];
export type KnownHostCandidate = components["schemas"]["KnownHostCandidate"];

export const KNOWN_HOSTS_DELETE_ACTION_KIND = "known_hosts.delete";
export const KNOWN_HOSTS_SCAN_ACTION_KIND = "known_hosts.scan";
export const KNOWN_HOSTS_ADD_ACTION_KIND = "known_hosts.add";

export type KnownHostAddition = Pick<
  KnownHostCandidate,
  "host" | "port" | "keyType" | "key"
>;

export type KnownHostsApi = {
  knownHosts(query: string): Promise<KnownHostsResponse>;
  deleteKnownHosts(
    entries: { line: number; digest: string }[],
    path: string,
  ): Promise<KnownHostsChangeResponse>;
  scanKnownHosts(host: string, port: number): Promise<KnownHostsScanResponse>;
  addKnownHost(
    candidate: KnownHostAddition,
    expectedFingerprint: string,
    acknowledged: boolean,
  ): Promise<KnownHostsChangeResponse>;
};

function validateKnownHosts(value: unknown): KnownHostsResponse {
  return validateOpenAPISchema<KnownHostsResponse>("KnownHostsResponse", value);
}

function validateChange(value: unknown): KnownHostsChangeResponse {
  return validateOpenAPISchema<KnownHostsChangeResponse>("KnownHostsChangeResponse", value);
}

function validateScan(value: unknown): KnownHostsScanResponse {
  return validateOpenAPISchema<KnownHostsScanResponse>("KnownHostsScanResponse", value);
}

// The OpenSSH known_hosts files: reading, scanning a host for its keys, and
// the token-guarded add and delete.
export const knownHostsApi: KnownHostsApi = {
  async knownHosts(query) {
    return validateKnownHosts(
      await apiClient.read(
        `/api/v1/known-hosts?query=${encodeURIComponent(query)}`,
      ),
    );
  },
  async deleteKnownHosts(entries, path) {
    const token = await issueAction(KNOWN_HOSTS_DELETE_ACTION_KIND, path);
    return validateChange(
      await postJSON<unknown>("/api/v1/known-hosts/delete", { entries }, token),
    );
  },
  async scanKnownHosts(host, port) {
    const token = await issueAction(KNOWN_HOSTS_SCAN_ACTION_KIND, host);
    return validateScan(
      await postJSON<unknown>(
        "/api/v1/known-hosts/scan",
        { host, port },
        token,
      ),
    );
  },
  async addKnownHost(candidate, expectedFingerprint, acknowledged) {
    const token = await issueAction(
      KNOWN_HOSTS_ADD_ACTION_KIND,
      candidate.host,
    );
    return validateChange(
      await postJSON<unknown>(
        "/api/v1/known-hosts/add",
        {
          host: candidate.host,
          port: candidate.port,
          keyType: candidate.keyType,
          key: candidate.key,
          expectedFingerprint,
          acknowledged,
        },
        token,
      ),
    );
  },
};
