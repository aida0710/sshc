import { apiClient } from "./client";
import { patchJSON, postJSON } from "./guards";
import { readOptions, type ReadScope } from "./readScope";
import { localApiBase } from "./remoteEngines";
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
export type PendingChanges = components["schemas"]["PendingChanges"];
export type PendingFileChange = components["schemas"]["PendingFileChange"];
export type Metadata = components["schemas"]["Metadata"];
export type GroupMetadata = components["schemas"]["GroupMetadata"];
export type GroupView = components["schemas"]["GroupView"];
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
export type UpdateConnectionTOTP = components["schemas"]["UpdateConnectionTOTP"];

// hostMetadataEditRequest は、接続 1 件の metadata だけを変える保存要求を作る。base は
// 画面が読み込んだときのその接続の metadata（無ければ識別子だけ）で、サーバーはいまの
// ディスクと違えば metadata_changed で断る。next が null ならその entry を消し、識別子が
// base と違えば別の接続へ付け直す。
export function hostMetadataEditRequest(base: HostMetadata, next: HostMetadata | null): EditRequest {
  return {
    kind: "metadata",
    path: base.identity.path,
    alias: base.identity.alias,
    hostMetadataBase: base,
    ...(next === null ? {} : { hostMetadata: next }),
  };
}




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

function validatePendingChanges(value: unknown): PendingChanges {
  return validateOpenAPISchema<PendingChanges>("PendingChanges", value);
}

function validateHistory(value: unknown): HistoryEntry[] {
  return validateOpenAPISchema<components["schemas"]["HistoryList"]>("HistoryList", value).entries;
}

// createConfigApi is the SSH configuration API of the engine at base (engineApiBase). The reads of a run target end
// with `reads` when the screen leaves it.
export function createConfigApi(base: string, reads?: ReadScope) {
  return {
    async overview(): Promise<Overview> {
      return validateOverview(await apiClient.read(`${base}/config/overview`, readOptions(reads)));
    },
    async host(path: string, alias: string): Promise<HostDetail> {
      const query = new URLSearchParams({ path, alias });
      return validateHostDetail(await apiClient.read(`${base}/config/host?${query.toString()}`));
    },
    async file(path: string): Promise<FileContents> {
      const query = new URLSearchParams({ path });
      return validateFileContents(await apiClient.read(`${base}/config/file?${query.toString()}`));
    },
    async preview(request: EditRequest): Promise<SavePreview> {
      return validateSavePreview(await postJSON<unknown>(`${base}/config/preview`, request));
    },
    async save(request: EditRequest): Promise<SaveResult> {
      return validateSaveResult(await postJSON<unknown>(`${base}/config/save`, request));
    },
    async createConnection(request: CreateConnectionRequest): Promise<CreateConnectionResponse> {
      return validateCreateConnectionResponse(await postJSON<unknown>(`${base}/connections`, request));
    },
    async updateConnection(request: UpdateConnectionRequest): Promise<SaveResult> {
      return validateSaveResult(await patchJSON<unknown>(`${base}/connections`, request));
    },
    async renameGroup(from: string, to: string): Promise<SaveResult> {
      return validateSaveResult(await postJSON<unknown>(`${base}/config/groups/rename`, { from, to }));
    },
    async deleteGroup(name: string, destination: string): Promise<SaveResult> {
      return validateSaveResult(await postJSON<unknown>(`${base}/config/groups/delete`, { name, destination }));
    },
    async history(): Promise<HistoryEntry[]> {
      return validateHistory(await apiClient.read(`${base}/history`));
    },
    async restore(transactionId: string, path: string): Promise<SaveResult> {
      return validateSaveResult(await postJSON<unknown>(`${base}/history/restore`, { transactionId, path }));
    },
    async pendingChanges(transactionId: string): Promise<PendingChanges> {
      return validatePendingChanges(await apiClient.read(`${base}/history/pending/${encodeURIComponent(transactionId)}`));
    },
    async recover(transactionId: string, action: "complete" | "rollback"): Promise<void> {
      await postJSON<unknown>(`${base}/history/recover`, { transactionId, action });
    },
  };
}

export const configApi = createConfigApi(localApiBase);

