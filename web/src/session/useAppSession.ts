import { useCallback, useEffect, useRef, useState } from "react";
import {
  apiClient,
  failureCode,
  whenLocked,
  whenRequestFailed,
  whenSessionEnded,
  type HealthResponse,
  type RequestFailureDiagnostic,
} from "../api/client";
import type { PasswordVaultStatus } from "../api/vault";
import type { SessionState } from "./bootstrap";
import {
  announceVaultLocked,
  observeVaultLocked,
} from "../secrets/vaultLockSignal";

export type AppSessionPhase =
  "starting" | "locked" | "ready" | "session-ended" | "error";

export type VaultRecheckPhase = "idle" | "checking" | "retrying";

type UseAppSessionOptions = Readonly<{
  bootstrap: () => Promise<SessionState>;
  health: () => Promise<HealthResponse>;
  vault: () => Promise<PasswordVaultStatus>;
}>;

export const vaultStatePollIntervalMs = 15_000;

export function useAppSession({
  bootstrap,
  health,
  vault,
}: UseAppSessionOptions) {
  const [state, setState] = useState<AppSessionPhase>("starting");
  const [vaultRecheck, setVaultRecheck] = useState<VaultRecheckPhase>("idle");
  const [failure, setFailure] = useState("");
  const [vaultExists, setVaultExists] = useState(false);
  const [passwordless, setPasswordless] = useState(false);
  // The check below reads this when the page comes back; its effect lives as long as the app is ready.
  const passwordlessNow = useRef(passwordless);
  passwordlessNow.current = passwordless;
  // The lock screen needs the engine's minimum to create a vault. It is an
  // engine constant, so the status read at start is enough.
  const [minPassphraseLength, setMinPassphraseLength] = useState<number | undefined>(undefined);
  const [version, setVersion] = useState("");
  const [requestFailure, setRequestFailure] =
    useState<RequestFailureDiagnostic | null>(null);
  const [vaultMigration, setVaultMigration] = useState<{
    from: number;
    to: number;
  } | null>(null);

  useEffect(() => {
    let active = true;
    void bootstrap()
      .then((sessionState) => {
        if (!active) return null;
        apiClient.setCSRF(sessionState.csrfToken);
        return health();
      })
      .then((result) => {
        if (!active || result === null) return null;
        setVersion(result.version);
        return vault();
      })
      .then((status) => {
        if (!active || status === null) return;
        setVaultExists(status.exists);
        setPasswordless(status.passwordless ?? false);
        setMinPassphraseLength(status.minPassphraseLength);
        setState(status.unlocked ? "ready" : "locked");
      })
      .catch((reason: unknown) => {
        console.error("sshc could not start its session", reason);
        if (!active) return;
        const code = failureCode(reason);
        if (
          (reason instanceof Error && reason.message === "session_expired") ||
          code === "session_required" ||
          code === "invalid_session" ||
          code === "invalid_csrf"
        ) {
          setState("session-ended");
          return;
        }
        setFailure(reason instanceof Error ? reason.message : String(reason));
        setState("error");
      });

    return () => {
      active = false;
      apiClient.clear();
    };
  }, [bootstrap, health, vault]);

  useEffect(() => {
    if (state !== "locked" || !vaultExists) return;
    let active = true;
    void vault().then((status) => {
      if (active) setPasswordless(status.passwordless ?? false);
    }).catch(() => undefined);
    return () => { active = false; };
  }, [state, vaultExists, vault]);

  const stateNow = useRef(state);
  stateNow.current = state;

  const lock = useCallback(() => {
    // The screen that creates a vault is locked too, and what it saves (the language) is refused: that says nothing
    // about a vault existing, and the screen stays as it is.
    if (stateNow.current === "locked") return;
    setVaultExists(true);
    setState("locked");
    announceVaultLocked();
  }, []);

  useEffect(() => {
    whenLocked(lock);
    const stopObserving = observeVaultLocked(() => {
      setVaultExists(true);
      setState("locked");
    });
    return () => {
      whenLocked(null);
      stopObserving();
    };
  }, [lock]);

  useEffect(() => {
    if (state !== "ready") return;
    let active = true;
    let checking = false;
    let holdUntilChecked = false;
    const checkVaultState = async (failClosed = false) => {
      if (failClosed) {
        holdUntilChecked = true;
        // Coming back again before a retry has succeeded confirms nothing, so what a failed check hid stays hidden.
        setVaultRecheck((phase) => (phase === "retrying" ? phase : "checking"));
      }
      if (checking) return;
      checking = true;
      try {
        const status = await vault();
        if (!active) return;
        setPasswordless(status.passwordless ?? false);
        if (!status.unlocked) {
          setVaultExists(status.exists);
          setState("locked");
          announceVaultLocked();
          return;
        }
        if (holdUntilChecked) {
          holdUntilChecked = false;
          setVaultRecheck("idle");
        }
      } catch {
        if (active && holdUntilChecked) setVaultRecheck("retrying");
      } finally {
        checking = false;
      }
    };
    const interval = window.setInterval(
      () => void checkVaultState(),
      vaultStatePollIntervalMs,
    );
    const checkWhenVisible = () => {
      // The engine never locks a passwordless vault by itself, so coming back has nothing to wait for.
      if (document.visibilityState === "visible") void checkVaultState(!passwordlessNow.current);
    };
    const checkWhenFocused = () => {
      if (document.visibilityState === "visible") void checkVaultState();
    };
    document.addEventListener("visibilitychange", checkWhenVisible);
    window.addEventListener("focus", checkWhenFocused);
    return () => {
      active = false;
      window.clearInterval(interval);
      document.removeEventListener("visibilitychange", checkWhenVisible);
      window.removeEventListener("focus", checkWhenFocused);
    };
  }, [state, vault]);

  useEffect(() => {
    whenSessionEnded(() => setState("session-ended"));
    return () => whenSessionEnded(null);
  }, []);

  useEffect(() => {
    whenRequestFailed(setRequestFailure);
    return () => whenRequestFailed(null);
  }, []);

  const markVaultExists = useCallback(() => setVaultExists(true), []);
  const openVault = useCallback((status?: PasswordVaultStatus) => {
    setPasswordless(status?.passwordless ?? false);
    if (
      typeof status?.migratedFromVersion === "number" &&
      typeof status.migratedToVersion === "number"
    ) {
      setVaultMigration({
        from: status.migratedFromVersion,
        to: status.migratedToVersion,
      });
    }
    setVaultExists(true);
    setVaultRecheck("idle");
    setState("ready");
  }, []);

  return {
    state,
    vaultRecheck,
    failure,
    vaultExists,
    passwordless,
    minPassphraseLength,
    version,
    requestFailure,
    vaultMigration,
    lock,
    markVaultExists,
    openVault,
    clearRequestFailure: () => setRequestFailure(null),
    clearVaultMigration: () => setVaultMigration(null),
  };
}
