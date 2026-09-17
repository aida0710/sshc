import { apiClient } from "./client";
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

// Hosts the engine connected to most recently, for the quick-connect lists.
export const recentConnectionsApi: RecentConnectionsApi = {
  async recentConnections() {
    return validateRecentConnections(
      await apiClient.read("/api/v1/connections/recent"),
    );
  },
};
