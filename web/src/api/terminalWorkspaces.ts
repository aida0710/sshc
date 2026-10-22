import { apiClient } from "./client";
import { patchJSON, postJSON } from "./guards";
import type { components } from "./schema";
import { validateOpenAPISchema } from "./validators.generated";

export type TerminalWorkspace = components["schemas"]["TerminalWorkspace"];
export type TerminalWorkspaceList = components["schemas"]["TerminalWorkspaceList"];
export type UpdateTerminalWorkspaceRequest = components["schemas"]["UpdateTerminalWorkspaceRequest"];

// The built-in workspaces the engine always has. They cannot be deleted, and
// an empty name means the screen shows its own name for them.
export const defaultWorkspaceId = "default";
export const agentsWorkspaceId = "agents";

export type TerminalWorkspacesApi = {
  list(): Promise<TerminalWorkspaceList>;
  create(name: string): Promise<TerminalWorkspace>;
  update(id: string, change: UpdateTerminalWorkspaceRequest): Promise<TerminalWorkspace>;
  remove(id: string): Promise<TerminalWorkspaceList>;
};

function validateList(value: unknown): TerminalWorkspaceList {
  return validateOpenAPISchema<TerminalWorkspaceList>("TerminalWorkspaceList", value);
}

function validateWorkspace(value: unknown): TerminalWorkspace {
  return validateOpenAPISchema<TerminalWorkspace>("TerminalWorkspace", value);
}

const workspacePath = (id: string) => `/api/v1/terminal/workspaces/${encodeURIComponent(id)}`;

// The engine's terminal workspaces: the named groups the session list is
// drawn in. Moving a session between them is a session operation
// (terminalSessionsApi.moveTerminalSession).
export const terminalWorkspacesApi: TerminalWorkspacesApi = {
  async list() {
    return validateList(await apiClient.read("/api/v1/terminal/workspaces"));
  },
  async create(name) {
    return validateWorkspace(await postJSON<unknown>("/api/v1/terminal/workspaces", { name }));
  },
  async update(id, change) {
    return validateWorkspace(await patchJSON<unknown>(workspacePath(id), change));
  },
  async remove(id) {
    return validateList(await apiClient.mutate<unknown>(workspacePath(id), { method: "DELETE" }));
  },
};
