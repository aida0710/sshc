import { apiClient } from "../../api/client";
import { asArray, asRecord, asString, jsonHeaders } from "../../api/guards";
import type { StoredNode } from "./layout";

export type SavedWorkspace = {
  id: string;
  name: string;
  layout: StoredNode;
  focusedPaneId: string;
  createdAt: string;
  updatedAt: string;
};

export type WorkspaceDefinition = Pick<SavedWorkspace, "name" | "layout" | "focusedPaneId">;

function node(value: unknown): StoredNode {
  const item = asRecord(value);
  if (item.pane !== undefined) {
    const pane = asRecord(item.pane);
    return { pane: { id: asString(pane.id), alias: asString(pane.alias) } };
  }
  const split = asRecord(item.split);
  const direction = asString(split.direction);
  if (direction !== "horizontal" && direction !== "vertical") throw new Error("invalid_response");
  const ratio = split.ratio;
  if (typeof ratio !== "number") throw new Error("invalid_response");
  return { split: { direction, ratio, first: node(split.first), second: node(split.second) } };
}

function workspace(value: unknown): SavedWorkspace {
  const item = asRecord(value);
  return {
    id: asString(item.id), name: asString(item.name), layout: node(item.layout),
    focusedPaneId: asString(item.focusedPaneId), createdAt: asString(item.createdAt), updatedAt: asString(item.updatedAt),
  };
}

export const workspaceApi = {
  async list(): Promise<SavedWorkspace[]> {
    const value = asRecord(await apiClient.read("/api/v1/workspaces"));
    return asArray(value.workspaces).map(workspace);
  },
  async create(definition: WorkspaceDefinition): Promise<SavedWorkspace> {
    return workspace(await apiClient.mutate<unknown>("/api/v1/workspaces", {
      method: "POST", headers: jsonHeaders, body: JSON.stringify(definition),
    }));
  },
  async update(id: string, definition: WorkspaceDefinition): Promise<SavedWorkspace> {
    return workspace(await apiClient.mutate<unknown>(`/api/v1/workspaces/${encodeURIComponent(id)}`, {
      method: "PUT", headers: jsonHeaders, body: JSON.stringify(definition),
    }));
  },
  async remove(id: string): Promise<void> {
    const response = await apiClient.send(`/api/v1/workspaces/${encodeURIComponent(id)}`, { method: "DELETE" });
    if (!response.ok) throw new Error("workspace_delete_failed");
  },
  async restore(id: string): Promise<SavedWorkspace> {
    const value = asRecord(await apiClient.mutate<unknown>(`/api/v1/workspaces/${encodeURIComponent(id)}/restore`, {
      method: "POST", headers: jsonHeaders, body: "{}",
    }));
    return workspace(value.workspace);
  },
};
