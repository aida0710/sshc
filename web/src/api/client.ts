import type { components } from "./schema";

export type HealthResponse = components["schemas"]["HealthResponse"];
export type Problem = components["schemas"]["Problem"];

export class ApiError extends Error {
  readonly code: string;
  readonly status: number;
  readonly problem: Problem | null;

  constructor(code: string, status: number, problem: Problem | null) {
    super(code);
    this.name = "ApiError";
    this.code = code;
    this.status = status;
    this.problem = problem;
  }
}

export function failureCode(error: unknown): string {
  return error instanceof ApiError ? error.code : "";
}

async function readProblem(response: Response): Promise<Problem | null> {
  try {
    const payload: unknown = await response.json();
    if (typeof payload !== "object" || payload === null) return null;
    const record = payload as Record<string, unknown>;
    if (typeof record.code !== "string" || typeof record.message !== "string") return null;
    return record as Problem;
  } catch {
    return null;
  }
}

let onLocked: (() => void) | null = null;
let onSessionEnded: (() => void) | null = null;

export function whenLocked(handler: (() => void) | null) {
  onLocked = handler;
}

export function whenSessionEnded(handler: (() => void) | null) {
  onSessionEnded = handler;
}

async function failure(response: Response): Promise<ApiError> {
  const problem = await readProblem(response);
  const code = problem?.code ?? "request_failed";
  if (code === "vault_locked") onLocked?.();
  return new ApiError(code, response.status, problem);
}

function validateHealth(value: unknown): HealthResponse {
  if (typeof value !== "object" || value === null) {
    throw new Error("invalid_health_response");
  }

  const record = value as Record<string, unknown>;
  if (record.status !== "ok" || typeof record.version !== "string" || record.version.length === 0) {
    throw new Error("invalid_health_response");
  }
  return { status: "ok", version: record.version };
}

let csrfToken: string | null = null;
let renewal: Readonly<{ token: string; attempt: symbol; promise: Promise<boolean> }> | null = null;

function validCSRF(value: unknown): value is { csrfToken: string } {
  if (typeof value !== "object" || value === null) return false;
  const token = (value as Record<string, unknown>).csrfToken;
  return typeof token === "string" && /^[A-Za-z0-9_-]{43}$/.test(token);
}

function endSession(expectedToken?: string) {
  if (expectedToken !== undefined && csrfToken !== expectedToken) return;
  csrfToken = null;
  onSessionEnded?.();
}

async function renewCSRF(expectedToken: string): Promise<boolean> {
  if (csrfToken !== expectedToken) return csrfToken !== null;
  if (renewal?.token === expectedToken) return renewal.promise;
  const attempt = Symbol("csrf-renewal");
  const promise = (async () => {
    try {
      const response = await fetch("/api/v1/session/renew", {
        method: "POST",
        credentials: "same-origin",
      });
      if (csrfToken !== expectedToken) return csrfToken !== null;
      if (!response.ok) {
        endSession(expectedToken);
        return false;
      }
      const payload: unknown = await response.json();
      if (!validCSRF(payload)) {
        endSession(expectedToken);
        return false;
      }
      csrfToken = payload.csrfToken;
      return true;
    } catch {
      endSession(expectedToken);
      return false;
    } finally {
      if (renewal?.attempt === attempt) renewal = null;
    }
  })();
  renewal = { token: expectedToken, attempt, promise };
  return promise;
}

async function sessionFailureCode(response: Response): Promise<string> {
  if (response.status !== 401 && response.status !== 403) return "";
  return (await readProblem(response.clone()))?.code ?? "";
}

async function requestWithSession(
  path: string,
  init: RequestInit,
  token: string,
  mayRenew = true,
): Promise<Response> {
  const headers = new Headers(init.headers);
  headers.set("X-SSHC-CSRF", token);
  const response = await fetch(path, { ...init, credentials: "same-origin", headers });
  const code = await sessionFailureCode(response);
  if (response.status === 401 && (code === "session_required" || code === "invalid_session")) {
    const fresh = csrfToken;
    if (mayRenew && fresh !== null && fresh !== token) return requestWithSession(path, init, fresh, false);
    endSession(token);
    return response;
  }
  if (response.status !== 403 || code !== "invalid_csrf") return response;
  if (!mayRenew) {
    endSession(token);
    return response;
  }

  // A concurrent request may already have replaced the token after this request left.
  // In that case retry with that token instead of rotating it again.
  if (csrfToken === token && !(await renewCSRF(token))) return response;
  const fresh = csrfToken;
  if (fresh === null) return response;
  // The security middleware rejects invalid_csrf before dispatching the handler, so
  // retrying a mutation here cannot repeat an operation that already ran.
  return requestWithSession(path, init, fresh, false);
}

export const apiClient = {
  setCSRF(token: string) {
    csrfToken = token;
  },
  clear() {
    csrfToken = null;
    renewal = null;
  },
  async health(): Promise<HealthResponse> {
    const response = await fetch("/api/v1/health", { credentials: "same-origin" });
    if (!response.ok) throw new Error("health_failed");
    return validateHealth(await response.json());
  },
  async read(path: string): Promise<unknown> {
    if (!csrfToken) throw new Error("csrf_unavailable");
    const response = await requestWithSession(path, {}, csrfToken);
    if (!response.ok) throw await failure(response);
    return response.json() as Promise<unknown>;
  },
  async send(path: string, init: RequestInit): Promise<Response> {
    const target = new URL(path, window.location.origin);
    if (target.origin !== window.location.origin) {
      throw new Error("cross_origin_api_mutation");
    }
    if (!csrfToken) throw new Error("csrf_unavailable");

    return requestWithSession(path, init, csrfToken);
  },
  async mutate<T>(path: string, init: RequestInit): Promise<T> {
    const response = await this.send(path, init);
    if (!response.ok) throw await failure(response);
    return response.json() as Promise<T>;
  },
};
