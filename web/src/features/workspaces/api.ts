import { apiClient } from "../../api/client";
import { jsonHeaders } from "../../api/guards";
import type { components } from "../../api/schema";
import { validateOpenAPISchema } from "../../api/validators.generated";
import type { StoredNode } from "./layout";

type WireWorkspace = components["schemas"]["TerminalWorkspace"];
type WireWorkspaceList = components["schemas"]["WorkspaceList"];
type WireRestorePlan = components["schemas"]["WorkspaceRestorePlan"];

export type SavedWorkspace = Omit<WireWorkspace, "layout"> & { layout: StoredNode };

export type WorkspaceDefinition = Pick<SavedWorkspace, "name" | "layout" | "focusedPaneId">;

function node(value: components["schemas"]["WorkspaceNode"]): StoredNode {
  if (value.pane !== undefined) {
    return { pane: {
      id: value.pane.id,
      alias: value.pane.alias,
      ...(value.pane.kind === "shell" ? { kind: value.pane.kind } : {}),
    } };
  }
  // OpenAPI's x-sshc-exactly-one validator guarantees one of pane/split.
  const split = value.split!;
  return { split: { direction: split.direction, ratio: split.ratio, first: node(split.first), second: node(split.second) } };
}

function workspace(value: unknown): SavedWorkspace {
  const item = validateOpenAPISchema<WireWorkspace>("TerminalWorkspace", value);
  return { ...item, layout: node(item.layout) };
}

export const workspaceApi = {
  async list(): Promise<SavedWorkspace[]> {
    const value = validateOpenAPISchema<WireWorkspaceList>("WorkspaceList", await apiClient.read("/api/v1/workspaces"));
    return value.workspaces.map(workspace);
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
    const value = validateOpenAPISchema<WireRestorePlan>("WorkspaceRestorePlan", await apiClient.mutate<unknown>(`/api/v1/workspaces/${encodeURIComponent(id)}/restore`, {
      method: "POST",
    }));
    return workspace(value.workspace);
  },
};
