import { ApiError, apiClient, type Problem } from "./client";


export function asRecord(value: unknown): Record<string, unknown> {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    throw new Error("invalid_response");
  }
  return value as Record<string, unknown>;
}

export function asArray(value: unknown): unknown[] {
  if (!Array.isArray(value)) throw new Error("invalid_response");
  return value;
}

export function asString(value: unknown): string {
  if (typeof value !== "string") throw new Error("invalid_response");
  return value;
}

export function asNumber(value: unknown): number {
  if (typeof value !== "number") throw new Error("invalid_response");
  return value;
}

export function asBoolean(value: unknown): boolean {
  if (typeof value !== "boolean") throw new Error("invalid_response");
  return value;
}

export const jsonHeaders = { "Content-Type": "application/json" } as const;

export function toProblem(error: unknown): Problem {
  if (error instanceof ApiError && error.problem !== null) return error.problem;
  if (error instanceof ApiError) return { code: error.code, message: "request rejected" };
  return { code: "request_failed", message: "request rejected" };
}

export async function issueAction(kind: string, target: string): Promise<string> {
  const response = await apiClient.mutate<unknown>("/api/v1/actions", {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({ kind, target }),
  });
  return asString(asRecord(response).token);
}

export type JSONRequest = {
  method: "POST" | "PUT" | "PATCH";
  body: unknown;
  // A confirmation token from issueAction, for operations the engine gates.
  actionToken?: string;
  // Problem codes the caller explains itself instead of the shared handler.
  locallyHandledCodes?: readonly string[];
};

// Every JSON mutation goes through here so the content type, the action
// header and the local-handling option are spelled once.
export function sendJSON<T>(path: string, request: JSONRequest): Promise<T> {
  const headers: Record<string, string> = { ...jsonHeaders };
  if (request.actionToken) headers["X-SSHC-Action"] = request.actionToken;
  return apiClient.mutate<T>(
    path,
    { method: request.method, headers, body: JSON.stringify(request.body) },
    request.locallyHandledCodes === undefined ? {} : { locallyHandledCodes: request.locallyHandledCodes },
  );
}

export function postJSON<T>(
  path: string,
  body: unknown,
  actionToken?: string,
  locallyHandledCodes?: readonly string[],
): Promise<T> {
  return sendJSON<T>(path, {
    method: "POST", body,
    ...(actionToken === undefined ? {} : { actionToken }),
    ...(locallyHandledCodes === undefined ? {} : { locallyHandledCodes }),
  });
}

export function putJSON<T>(path: string, body: unknown, locallyHandledCodes?: readonly string[]): Promise<T> {
  return sendJSON<T>(path, { method: "PUT", body, ...(locallyHandledCodes === undefined ? {} : { locallyHandledCodes }) });
}

export function patchJSON<T>(path: string, body: unknown, actionToken?: string): Promise<T> {
  return sendJSON<T>(path, { method: "PATCH", body, ...(actionToken === undefined ? {} : { actionToken }) });
}

export async function postEmpty<T>(path: string): Promise<T> {
  return apiClient.mutate<T>(path, { method: "POST" });
}
