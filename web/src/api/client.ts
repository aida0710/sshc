import type { components } from "./schema";
import { clearSessionCSRF } from "../session/bootstrap";
import { validateAPIRequest, validateAPIResponse, validateOpenAPISchema } from "./validators.generated";
import { isWorkspaceRefusal } from "./workspaceRefusals";

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
  signal?: AbortSignal;
  locallyHandledCodes?: readonly string[];
}>;

export type MutationOptions = RequestFailureOptions & Readonly<{
  // The status under which the endpoint answers a refusal with its declared
  // response body (the blockers of a key relocation or restore) instead of a
  // Problem. That body is returned like a success; a Problem under the same
  // status still fails as an ApiError.
  refusalStatus?: number;
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

// failurePath は、engine が失敗の原因として示したファイルのパスを返す。示していなければ空。
export function failurePath(error: unknown): string {
  return error instanceof ApiError ? (error.problem?.path ?? "") : "";
}

async function readProblem(response: Response): Promise<Problem | null> {
  try {
    const payload: unknown = await response.json();
    return validateOpenAPISchema<Problem>("Problem", payload);
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
  // alertが二重になり、画面readerにも同じ失敗を二度伝えてしまう。workspaceを使えない
  // 拒否だけは、どの画面も説明しないので共通通知に出す。
  if (diagnostic.status >= 400 && diagnostic.status < 500 && !isWorkspaceRefusal(diagnostic.code)) return;
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

async function isDeclaredRefusal(response: Response, options: MutationOptions): Promise<boolean> {
  return response.status === options.refusalStatus && (await readProblem(response.clone())) === null;
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
  try {
    return validateOpenAPISchema<HealthResponse>("HealthResponse", value);
  } catch {
    throw new Error("invalid_health_response");
  }
}

let csrfToken: string | null = null;

function apiPath(path: string): string {
  return new URL(path, window.location.origin).pathname;
}

async function validatedJSON<T>(response: Response, method: string, path: string): Promise<T> {
  const payload: unknown = await response.json();
  return validateAPIResponse<T>(method, apiPath(path), response.status, payload);
}

function validateJSONRequest(method: string, path: string, init: RequestInit): void {
  const headers = new Headers(init.headers);
  if (!headers.get("Content-Type")?.toLowerCase().startsWith("application/json") || typeof init.body !== "string") return;
  let payload: unknown;
  try {
    payload = JSON.parse(init.body) as unknown;
  } catch {
    throw new Error("invalid_request");
  }
  validateAPIRequest(method, apiPath(path), payload);
}

function endSession(rejectedToken: string) {
  // A token installed after this request left belongs to a newer session.
  if (csrfToken !== rejectedToken) return;
  csrfToken = null;
  clearSessionCSRF();
  onSessionEnded?.();
}

async function sessionFailureCode(response: Response): Promise<string> {
  if (response.status !== 401 && response.status !== 403) return "";
  return (await readProblem(response.clone()))?.code ?? "";
}

async function requestWithSession(path: string, init: RequestInit, token: string): Promise<Response> {
  const headers = new Headers(init.headers);
  headers.set("X-SSHC-CSRF", token);
  const response = await fetch(path, { ...init, credentials: "same-origin", headers });
  const code = await sessionFailureCode(response);
  const sessionMissing = response.status === 401 && (code === "session_required" || code === "invalid_session");
  // The engine renews a token only after verifying it (POST /api/v1/session/renew
  // checks X-SSHC-CSRF like any other request), so a rejected token cannot be
  // exchanged for a new one. Reloading the page recovers through the registered
  // browser token instead.
  const tokenRejected = response.status === 403 && code === "invalid_csrf";
  if (sessionMissing || tokenRejected) endSession(token);
  return response;
}

export const apiClient = {
  setCSRF(token: string) {
    csrfToken = token;
  },
  clear() {
    csrfToken = null;
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
    // Keep the stable health-specific diagnostic while the actual contract is
    // still generated from HealthResponse in OpenAPI.
    return validateHealth(await response.json());
  },
  async read(path: string, options: RequestFailureOptions = {}): Promise<unknown> {
    return validatedJSON<unknown>(await this.readResponse(path, options), "GET", path);
  },
  // readResponse is read for a body that is not JSON (a file preview): the
  // caller reads the body and its headers, and a failure is the same ApiError.
  async readResponse(path: string, options: RequestFailureOptions = {}): Promise<Response> {
    if (!csrfToken) throw new Error("csrf_unavailable");
    let response: Response;
    try {
      response = await requestWithSession(path, options.signal === undefined ? {} : { signal: options.signal }, csrfToken);
    } catch (error) {
      if (!options.signal?.aborted && !options.locallyHandledCodes?.includes("network_request_failed")) notifyNetworkFailure("GET", path);
      throw error;
    }
    if (!response.ok) throw await failure(response, "GET", path, options);
    return response;
  },
  async send(path: string, init: RequestInit, options: RequestFailureOptions = {}): Promise<Response> {
    const target = new URL(path, window.location.origin);
    if (target.origin !== window.location.origin) {
      throw new Error("cross_origin_api_mutation");
    }
    if (!csrfToken) throw new Error("csrf_unavailable");

    const method = init.method ?? "POST";
    validateJSONRequest(method, path, init);
    let response: Response;
    try {
      response = await requestWithSession(path, init, csrfToken);
    } catch (error) {
      notifyNetworkFailure(method, path);
      throw error;
    }
    if (!response.ok) {
      const problem = await readProblem(response.clone());
      if (!options.locallyHandledCodes?.includes(problem?.code ?? "request_failed")) {
        notifyResponseFailure(response, problem, method, path);
      }
    }
    return response;
  },
  async mutate<T>(path: string, init: RequestInit, options: MutationOptions = {}): Promise<T> {
    const method = init.method ?? "POST";
    const response = await this.send(path, init, options);
    if (!response.ok && !(await isDeclaredRefusal(response, options))) throw await failure(response, method, path, options);
    if (response.status === 204) {
      return validateAPIResponse<T>(method, apiPath(path), response.status, undefined);
    }
    return validatedJSON<T>(response, method, path);
  },
};
