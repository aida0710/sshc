import { apiClient } from "./client";
import type { components } from "./schema";
import { validateOpenAPISchema } from "./validators.generated";

export type Overview = components["schemas"]["Overview"];
export type HostEntry = components["schemas"]["HostEntry"];
export type HostIdentity = components["schemas"]["HostIdentity"];
export type HostDetail = components["schemas"]["HostDetail"];
export type HostForm = components["schemas"]["HostForm"];
export type FormField = components["schemas"]["FormField"];
export type FieldEdit = components["schemas"]["FieldEdit"];
export type EditRequest = components["schemas"]["EditRequest"];
export type SavePreview = components["schemas"]["SavePreview"];
export type SaveResult = components["schemas"]["SaveResult"];
export type FileContents = components["schemas"]["FileContents"];
export type FileNode = components["schemas"]["FileNode"];
export type FileDiff = components["schemas"]["FileDiff"];
export type DiffLine = components["schemas"]["DiffLine"];
export type ConflictReport = components["schemas"]["ConflictReport"];
export type HistoryEntry = components["schemas"]["HistoryEntry"];
export type PendingTransaction = components["schemas"]["PendingTransaction"];
export type Metadata = components["schemas"]["Metadata"];
export type GroupMetadata = components["schemas"]["GroupMetadata"];
export type HostMetadata = components["schemas"]["HostMetadata"];
export type Notice = components["schemas"]["Notice"];
export type Diagnostic = components["schemas"]["Diagnostic"];
export type EffectiveDiff = components["schemas"]["EffectiveDiff"];
export type CreateConnectionRequest = components["schemas"]["CreateConnectionRequest"];
export type CreateConnectionAuthentication = components["schemas"]["CreateConnectionAuthentication"];
export type CreateConnectionResponse = components["schemas"]["CreateConnectionResponse"];
export type UpdateConnectionRequest = components["schemas"]["UpdateConnectionRequest"];
export type UpdateConnectionPassword = components["schemas"]["UpdateConnectionPassword"];
export type UpdateConnectionKeyPassphrase = components["schemas"]["UpdateConnectionKeyPassphrase"];




function validateOverview(value: unknown): Overview {
  return validateOpenAPISchema<Overview>("Overview", value);
}

function validateHostDetail(value: unknown): HostDetail {
  return validateOpenAPISchema<HostDetail>("HostDetail", value);
}

function validateFileContents(value: unknown): FileContents {
  return validateOpenAPISchema<FileContents>("FileContents", value);
}

function validateSavePreview(value: unknown): SavePreview {
  return validateOpenAPISchema<SavePreview>("SavePreview", value);
}

function validateSaveResult(value: unknown): SaveResult {
  return validateOpenAPISchema<SaveResult>("SaveResult", value);
}

function validateCreateConnectionResponse(value: unknown): CreateConnectionResponse {
  return validateOpenAPISchema<CreateConnectionResponse>("CreateConnectionResponse", value);
}

function validateHistory(value: unknown): HistoryEntry[] {
  return validateOpenAPISchema<components["schemas"]["HistoryList"]>("HistoryList", value).entries;
}

function mutateJSON<T>(path: string, method: "POST" | "PATCH", body: unknown): Promise<T> {
  return apiClient.mutate<T>(path, {
    method,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

function postJSON<T>(path: string, body: unknown): Promise<T> {
  return mutateJSON<T>(path, "POST", body);
}

export const configApi = {
  async overview(): Promise<Overview> {
    return validateOverview(await apiClient.read("/api/v1/config/overview"));
  },
  async host(path: string, alias: string): Promise<HostDetail> {
    const query = new URLSearchParams({ path, alias });
    return validateHostDetail(await apiClient.read(`/api/v1/config/host?${query.toString()}`));
  },
  async file(path: string): Promise<FileContents> {
    const query = new URLSearchParams({ path });
    return validateFileContents(await apiClient.read(`/api/v1/config/file?${query.toString()}`));
  },
  async preview(request: EditRequest): Promise<SavePreview> {
    return validateSavePreview(await postJSON<unknown>("/api/v1/config/preview", request));
  },
  async save(request: EditRequest): Promise<SaveResult> {
    return validateSaveResult(await postJSON<unknown>("/api/v1/config/save", request));
  },
  async createConnection(request: CreateConnectionRequest): Promise<CreateConnectionResponse> {
    return validateCreateConnectionResponse(await postJSON<unknown>("/api/v1/connections", request));
  },
  async updateConnection(request: UpdateConnectionRequest): Promise<SaveResult> {
    return validateSaveResult(await mutateJSON<unknown>("/api/v1/connections", "PATCH", request));
  },
  async renameGroup(from: string, to: string): Promise<SaveResult> {
    return validateSaveResult(await postJSON<unknown>("/api/v1/config/groups/rename", { from, to }));
  },
  async deleteGroup(name: string, destination: string): Promise<SaveResult> {
    return validateSaveResult(await postJSON<unknown>("/api/v1/config/groups/delete", { name, destination }));
  },
  async history(): Promise<HistoryEntry[]> {
    return validateHistory(await apiClient.read("/api/v1/history"));
  },
  async restore(transactionId: string, path: string): Promise<SaveResult> {
    return validateSaveResult(await postJSON<unknown>("/api/v1/history/restore", { transactionId, path }));
  },
  async recover(transactionId: string, action: "complete" | "rollback"): Promise<void> {
    await postJSON<unknown>("/api/v1/history/recover", { transactionId, action });
  },
};
