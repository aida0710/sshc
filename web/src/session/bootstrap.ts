import type { components } from "../api/schema";
import { validateOpenAPISchema } from "../api/validators.generated";
import { localStorageKeys, sessionStorageKeys } from "../ui/browserStorageKeys";

type BootstrapResponse = components["schemas"]["BootstrapResponse"];

export type SessionState = Readonly<{ csrfToken: string }>;

// The engine mints the bootstrap, CSRF, and browser registration tokens the
// same way (token in internal/session/manager.go, and internal/browserauth
// for the registration): 32 random bytes in unpadded base64url, which is
// always 43 characters. One shape check covers all three; only the engine
// decides whether a token is valid.
const engineTokenLength = 43;
const engineTokenPattern = new RegExp(`^[A-Za-z0-9_-]{${engineTokenLength}}$`);

function isWellFormedToken(value: unknown): value is string {
  return typeof value === "string" && engineTokenPattern.test(value);
}

export function loadSessionCSRF(storage: Pick<Storage, "getItem" | "removeItem"> = window.sessionStorage): string {
  try {
    const value = storage.getItem(sessionStorageKeys.sessionCSRF);
    if (isWellFormedToken(value)) return value;
    if (value !== null) storage.removeItem(sessionStorageKeys.sessionCSRF);
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
  if (!isWellFormedToken(value)) return;
  try {
    storage.setItem(sessionStorageKeys.sessionCSRF, value);
  } catch {
    // See loadSessionCSRF: failure only removes reload continuity.
  }
}

export function clearSessionCSRF(storage: Pick<Storage, "removeItem"> = window.sessionStorage): void {
  try {
    storage.removeItem(sessionStorageKeys.sessionCSRF);
  } catch {
    // There is no persisted token to clear when storage is unavailable.
  }
}

export function loadBrowserToken(storage: Pick<Storage, "getItem" | "removeItem"> = window.localStorage): string {
  try {
    const value = storage.getItem(localStorageKeys.browserRegistration);
    if (isWellFormedToken(value)) return value;
    if (value !== null) storage.removeItem(localStorageKeys.browserRegistration);
  } catch {
    // A browser with disabled local storage can still enter through `sshc open`,
    // but cannot recover a session after an engine restart.
  }
  return "";
}

function storeBrowserToken(value: string, storage: Pick<Storage, "setItem"> = window.localStorage): void {
  if (!isWellFormedToken(value)) return;
  try {
    storage.setItem(localStorageKeys.browserRegistration, value);
  } catch {
    // The one-time session remains usable even if persistent enrolment is blocked.
  }
}

export function clearBrowserToken(storage: Pick<Storage, "removeItem"> = window.localStorage): void {
  try {
    storage.removeItem(localStorageKeys.browserRegistration);
  } catch {
    // Nothing persisted, nothing to clear.
  }
}

function isBootstrapResponse(value: unknown): value is BootstrapResponse {
  try {
    validateOpenAPISchema<BootstrapResponse>("BootstrapResponse", value);
    return true;
  } catch {
    return false;
  }
}

// adoptSession keeps the tokens the engine answered a bootstrap, renewal, or
// recovery with, and returns the session they open. Every answer carries a new
// CSRF token. Bootstrap and recovery also issue a browser registration, and a
// recovery rotates it: the old one stops working shortly after, so the
// replacement is stored before anything else runs.
async function adoptSession(response: Response): Promise<SessionState> {
  const payload: unknown = await response.json();
  if (!isBootstrapResponse(payload)) throw new Error("invalid_bootstrap_response");
  if (payload.browserToken !== undefined) storeBrowserToken(payload.browserToken);
  storeSessionCSRF(payload.csrfToken);
  return { csrfToken: payload.csrfToken };
}

async function recoverSession(fetcher: typeof fetch): Promise<SessionState> {
  const browserToken = loadBrowserToken();
  if (browserToken === "") throw new Error("session_expired");
  const recovered = await fetcher("/api/v1/session/recover", {
    method: "POST",
    credentials: "same-origin",
    headers: { "X-SSHC-Browser": browserToken },
  });
  if (!recovered.ok) {
    // The engine refused the registration: it was rotated elsewhere, expired, or
    // revoked. Keeping it would only repeat the refusal on every reload.
    if (recovered.status === 401) clearBrowserToken();
    throw new Error("session_expired");
  }
  return adoptSession(recovered);
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
        try {
          return await adoptSession(renewed);
        } catch (error) {
          clearSessionCSRF();
          throw error;
        }
      }
      clearSessionCSRF();
    }
    return recoverSession(fetcher);
  }

  if (!isWellFormedToken(bootstrap)) {
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
  if (!response.ok) {
    // A bootstrap link is single-use, so a reopened one (browser history, a
    // second click) is rejected. A browser this engine already registered can
    // still come in through its registration instead of a hard error.
    if ((response.status === 401 || response.status === 409) && loadBrowserToken() !== "") {
      return recoverSession(fetcher);
    }
    throw new Error("bootstrap_rejected");
  }
  return adoptSession(response);
}
