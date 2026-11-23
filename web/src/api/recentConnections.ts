import { apiClient } from "./client";
import { readOptions, type ReadScope } from "./readScope";
import { localApiBase } from "./remoteEngines";
import type { components } from "./schema";
import { validateOpenAPISchema } from "./validators.generated";

export type RecentConnection = components["schemas"]["RecentConnection"];
export type RecentConnectionList = components["schemas"]["RecentConnectionList"];

export type RecentConnectionsApi = {
  recentConnections(): Promise<RecentConnectionList>;
};

function validateRecentConnections(value: unknown): RecentConnectionList {
  return validateOpenAPISchema<RecentConnectionList>("RecentConnectionList", value);
}

// createRecentConnectionsApi reads the hosts the engine at base (engineApiBase) connected to most recently, for the
// quick-connect lists. The reads of a run target end with `reads` when the screen leaves it.
export function createRecentConnectionsApi(base: string, reads?: ReadScope): RecentConnectionsApi {
  return {
    async recentConnections() {
      return validateRecentConnections(await apiClient.read(`${base}/connections/recent`, readOptions(reads)));
    },
  };
}

export const recentConnectionsApi = createRecentConnectionsApi(localApiBase);
