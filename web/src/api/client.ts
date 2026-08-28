import type { components } from "./schema";
import { clearSessionCSRF, storeSessionCSRF } from "../session/bootstrap";

export type HealthResponse = components["schemas"]["HealthResponse"];
export type Problem = components["schemas"]["Problem"];

export type RequestFailureDiagnostic = Readonly<{
  code: string;
  status: number;
  method: string;
  path: string;
  detail?: string;
}>;

type RequestFailureOptions = Readonly<{
  locallyHandledCodes?: readonly string[];
}>;

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
let onRequestFailed: ((diagnostic: RequestFailureDiagnostic) => void) | null = null;
const reportedResponses = new WeakSet<Response>();

export function whenLocked(handler: (() => void) | null) {
  onLocked = handler;
}

export function whenSessionEnded(handler: (() => void) | null) {
  onSessionEnded = handler;
}

export function whenRequestFailed(handler: ((diagnostic: RequestFailureDiagnostic) => void) | null) {
  onRequestFailed = handler;
}

function diagnosticPath(path: string): string {
  try {
    return new URL(path, window.location.origin).pathname;
  } catch {
    return "/api";
  }
}

function notifyFailure(diagnostic: RequestFailureDiagnostic) {
  if (["vault_locked", "session_required", "invalid_session", "invalid_csrf"].includes(diagnostic.code)) return;
  // 4xxは各操作画面が入力不備や競合を具体的に説明する。共通通知まで重ねると
  // alertが二重になり、画面readerにも同じ失敗を二度伝えてしまう。
  if (diagnostic.status >= 400 && diagnostic.status < 500) return;
  // 更新確認は任意のbackground taskであり、製品操作の失敗ではない。
  if (diagnostic.code === "update_check_failed") return;
  onRequestFailed?.(diagnostic);
}

function notifyNetworkFailure(method: string, path: string) {
  notifyFailure({ code: "network_request_failed", status: 0, method, path: diagnosticPath(path) });
}

function notifyResponseFailure(
  response: Response,
  problem: Problem | null,
  method: string,
  path: string,
) {
  if (reportedResponses.has(response)) return;
  reportedResponses.add(response);
  const code = problem?.code ?? "request_failed";
  const detail = problem?.detail;
  notifyFailure({
    code,
    status: response.status,
    method,
    path: diagnosticPath(path),
    ...(typeof detail === "string" && detail !== "" ? { detail } : {}),
  });
}

async function failure(
  response: Response,
  method: string,
  path: string,
  options: RequestFailureOptions = {},
): Promise<ApiError> {
  const problem = await readProblem(response);
  const code = problem?.code ?? "request_failed";
  if (code === "vault_locked") onLocked?.();
  if (!options.locallyHandledCodes?.includes(code)) {
    notifyResponseFailure(response, problem, method, path);
  }
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
  clearSessionCSRF();
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
        headers: { "X-SSHC-CSRF": expectedToken },
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
      storeSessionCSRF(payload.csrfToken);
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
    clearSessionCSRF();
  },
  async health(): Promise<HealthResponse> {
    let response: Response;
    try {
      response = await fetch("/api/v1/health", { credentials: "same-origin" });
    } catch (error) {
      notifyNetworkFailure("GET", "/api/v1/health");
      throw error;
    }
    if (!response.ok) throw await failure(response, "GET", "/api/v1/health");
    return validateHealth(await response.json());
  },
  async read(path: string, options: RequestFailureOptions = {}): Promise<unknown> {
    if (!csrfToken) throw new Error("csrf_unavailable");
    let response: Response;
    try {
      response = await requestWithSession(path, {}, csrfToken);
    } catch (error) {
      notifyNetworkFailure("GET", path);
      throw error;
    }
    if (!response.ok) throw await failure(response, "GET", path, options);
    return response.json() as Promise<unknown>;
  },
  async send(path: string, init: RequestInit): Promise<Response> {
    const target = new URL(path, window.location.origin);
    if (target.origin !== window.location.origin) {
      throw new Error("cross_origin_api_mutation");
    }
    if (!csrfToken) throw new Error("csrf_unavailable");

    const method = init.method ?? "POST";
    let response: Response;
    try {
      response = await requestWithSession(path, init, csrfToken);
    } catch (error) {
      notifyNetworkFailure(method, path);
      throw error;
    }
    if (!response.ok) {
      notifyResponseFailure(response, await readProblem(response.clone()), method, path);
    }
    return response;
  },
  async mutate<T>(path: string, init: RequestInit): Promise<T> {
    const method = init.method ?? "POST";
    const response = await this.send(path, init);
    if (!response.ok) throw await failure(response, method, path);
    return response.json() as Promise<T>;
  },
};
