import { useCallback, useEffect, useState } from "react";
import {
  apiClient,
  failureCode,
  whenLocked,
  whenRequestFailed,
  whenSessionEnded,
  type HealthResponse,
  type RequestFailureDiagnostic,
} from "../api/client";
import type { PasswordVaultStatus } from "../api/integrations";
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

  const lock = useCallback(() => {
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
    let concealUntilChecked = false;
    const checkVaultState = async (failClosed = false) => {
      if (failClosed) {
        concealUntilChecked = true;
        setVaultRecheck("checking");
      }
      if (checking) return;
      checking = true;
      try {
        const status = await vault();
        if (!active) return;
        if (!status.unlocked) {
          setVaultExists(status.exists);
          setState("locked");
          announceVaultLocked();
          return;
        }
        if (concealUntilChecked) {
          concealUntilChecked = false;
          setVaultRecheck("idle");
        }
      } catch {
        if (active && concealUntilChecked) setVaultRecheck("retrying");
      } finally {
        checking = false;
      }
    };
    const interval = window.setInterval(
      () => void checkVaultState(),
      vaultStatePollIntervalMs,
    );
    const checkWhenVisible = () => {
      if (document.visibilityState === "visible") void checkVaultState(true);
    };
    document.addEventListener("visibilitychange", checkWhenVisible);
    window.addEventListener("focus", checkWhenVisible);
    return () => {
      active = false;
      window.clearInterval(interval);
      document.removeEventListener("visibilitychange", checkWhenVisible);
      window.removeEventListener("focus", checkWhenVisible);
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
