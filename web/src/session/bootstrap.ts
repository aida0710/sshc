import type { components } from "../api/schema";

type BootstrapResponse = components["schemas"]["BootstrapResponse"];

export type SessionState = Readonly<{ csrfToken: string }>;

const csrfStorageKey = "sshc.session.csrf";
const browserStorageKey = "sshc.browser.registration.v1";

function csrfToken(value: unknown): value is string {
  return typeof value === "string" && /^[A-Za-z0-9_-]{43}$/.test(value);
}

export function loadSessionCSRF(storage: Pick<Storage, "getItem" | "removeItem"> = window.sessionStorage): string {
  try {
    const value = storage.getItem(csrfStorageKey);
    if (csrfToken(value)) return value;
    if (value !== null) storage.removeItem(csrfStorageKey);
  } catch {
    // Storage can be disabled. The current page can still use its in-memory token,
    // but a reload must return through the one-time bootstrap path.
  }
  return "";
}

export function storeSessionCSRF(
  value: string,
  storage: Pick<Storage, "setItem"> = window.sessionStorage,
): void {
  if (!csrfToken(value)) return;
  try {
    storage.setItem(csrfStorageKey, value);
  } catch {
    // See loadSessionCSRF: failure only removes reload continuity.
  }
}

export function clearSessionCSRF(storage: Pick<Storage, "removeItem"> = window.sessionStorage): void {
  try {
    storage.removeItem(csrfStorageKey);
  } catch {
    // There is no persisted token to clear when storage is unavailable.
  }
}

function loadBrowserToken(storage: Pick<Storage, "getItem" | "removeItem"> = window.localStorage): string {
  try {
    const value = storage.getItem(browserStorageKey);
    if (csrfToken(value)) return value;
    if (value !== null) storage.removeItem(browserStorageKey);
  } catch {
    // A browser with disabled local storage can still enter through `sshc open`,
    // but cannot recover a session after an engine restart.
  }
  return "";
}

function storeBrowserToken(value: string, storage: Pick<Storage, "setItem"> = window.localStorage): void {
  if (!csrfToken(value)) return;
  try {
    storage.setItem(browserStorageKey, value);
  } catch {
    // The one-time session remains usable even if persistent enrolment is blocked.
  }
}

function isBootstrapResponse(value: unknown): value is BootstrapResponse {
  if (typeof value !== "object" || value === null) return false;
  const token = (value as Record<string, unknown>).csrfToken;
  const browserToken = (value as Record<string, unknown>).browserToken;
  return csrfToken(token) && (browserToken === undefined || csrfToken(browserToken));
}

async function recoverSession(fetcher: typeof fetch): Promise<SessionState> {
  const browserToken = loadBrowserToken();
  if (browserToken === "") throw new Error("session_expired");
  const recovered = await fetcher("/api/v1/session/recover", {
    method: "POST",
    credentials: "same-origin",
    headers: { "X-SSHC-Browser": browserToken },
  });
  if (!recovered.ok) throw new Error("session_expired");
  const payload: unknown = await recovered.json();
  if (!isBootstrapResponse(payload)) throw new Error("invalid_bootstrap_response");
  storeSessionCSRF(payload.csrfToken);
  return { csrfToken: payload.csrfToken };
}

export async function bootstrapSession(
  location: Pick<Location, "hash" | "pathname" | "search">,
  history: Pick<History, "replaceState">,
  fetcher: typeof fetch,
): Promise<SessionState> {
  const params = new URLSearchParams(location.hash.replace(/^#/, ""));
  const bootstrap = params.get("bootstrap") ?? "";

  if (bootstrap === "") {
    const current = loadSessionCSRF();
    if (current !== "") {
      const renewed = await fetcher("/api/v1/session/renew", {
        method: "POST",
        credentials: "same-origin",
        headers: { "X-SSHC-CSRF": current },
      });
      if (renewed.ok) {
        const payload: unknown = await renewed.json();
        if (!isBootstrapResponse(payload)) {
          clearSessionCSRF();
          throw new Error("invalid_bootstrap_response");
        }
        storeSessionCSRF(payload.csrfToken);
        return { csrfToken: payload.csrfToken };
      }
      clearSessionCSRF();
    }
    return recoverSession(fetcher);
  }

  if (!/^[A-Za-z0-9_-]{43}$/.test(bootstrap)) {
    throw new Error("invalid_bootstrap_fragment");
  }

  history.replaceState(null, "", `${location.pathname}${location.search}`);
  const headers: Record<string, string> = { "X-SSHC-Bootstrap": bootstrap };
  const browserToken = loadBrowserToken();
  if (browserToken !== "") headers["X-SSHC-Browser"] = browserToken;
  const response = await fetcher("/api/v1/session/bootstrap", {
    method: "POST",
    credentials: "same-origin",
    headers,
  });
  if (!response.ok) throw new Error("bootstrap_rejected");

  const payload: unknown = await response.json();
  if (!isBootstrapResponse(payload)) throw new Error("invalid_bootstrap_response");
  if (payload.browserToken !== undefined) storeBrowserToken(payload.browserToken);
  storeSessionCSRF(payload.csrfToken);
  return { csrfToken: payload.csrfToken };
}
