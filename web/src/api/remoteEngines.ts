import { apiClient } from "./client";
import { postEmpty, postJSON } from "./guards";
import type { components } from "./schema";
import { validateOpenAPISchema } from "./validators.generated";

export type RemoteEngineList = components["schemas"]["RemoteEngineList"];
export type RemoteEngineStatus = components["schemas"]["RemoteEngineStatus"];
export type RemoteEngineEntrance = components["schemas"]["RemoteEngineEntrance"];
export type RemoteEngineAction = components["schemas"]["RemoteEngineActionRequest"]["action"];
export type RemoteEngineActionResult = components["schemas"]["RemoteEngineActionResult"];

export type RemoteEnginesApi = {
  list(): Promise<RemoteEngineList>;
  add(alias: string): Promise<void>;
  remove(alias: string): Promise<RemoteEngineList>;
  status(alias: string): Promise<RemoteEngineStatus>;
  open(alias: string): Promise<RemoteEngineEntrance>;
  control(alias: string, action: RemoteEngineAction): Promise<RemoteEngineActionResult>;
};

const enginePath = (alias: string) => `/api/v1/engines/${encodeURIComponent(alias)}`;

// The engines on SSH hosts added as run targets. This machine's engine
// forwards their terminal API (see engineTerminalBase).
export const remoteEnginesApi: RemoteEnginesApi = {
  async list() {
    return validateOpenAPISchema<RemoteEngineList>("RemoteEngineList", await apiClient.read("/api/v1/engines"));
  },
  async add(alias) {
    validateOpenAPISchema("RemoteEngine", await postJSON<unknown>("/api/v1/engines", { alias }));
  },
  async remove(alias) {
    return validateOpenAPISchema<RemoteEngineList>("RemoteEngineList", await apiClient.mutate<unknown>(enginePath(alias), { method: "DELETE" }));
  },
  async status(alias) {
    return validateOpenAPISchema<RemoteEngineStatus>("RemoteEngineStatus", await apiClient.read(`${enginePath(alias)}/status`));
  },
  async open(alias) {
    return validateOpenAPISchema<RemoteEngineEntrance>("RemoteEngineEntrance", await postEmpty<unknown>(`${enginePath(alias)}/open`));
  },
  async control(alias, action) {
    return validateOpenAPISchema<RemoteEngineActionResult>("RemoteEngineActionResult", await postJSON<unknown>(`${enginePath(alias)}/control`, { action }));
  },
};

// engineTerminalBase is the terminal API of a run-target engine, as this
// machine's engine forwards it; engineStreamPath is its terminal stream.
export function engineTerminalBase(alias: string): string {
  return `${enginePath(alias)}/terminal`;
}

export function engineStreamPath(alias: string): string {
  return `/engines/${encodeURIComponent(alias)}/terminal/stream`;
}
