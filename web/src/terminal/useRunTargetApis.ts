import { useEffect, useMemo } from "react";
import { createReadScope } from "../api/readScope";
import { engineTerminalBase } from "../api/remoteEngines";
import { createTerminalRestoreApi, terminalRestoreApi } from "../api/terminalRestore";
import { runTargetTerminalSessionsApi, terminalSessionsApi } from "../api/terminalSessions";
import { createTerminalWorkspacesApi, terminalWorkspacesApi } from "../api/terminalWorkspaces";

// useRunTargetApis gives the terminal API of the run target: this machine's own, or the one this machine's engine
// forwards for an engine on an SSH host. A new API is a new engine to the hooks that use it, which drop what they
// showed and read it anew; that is wanted once a run target's Vault is unlocked, so the API is built again for each
// unlock (vaultRevision). Leaving a run target ends the reads still waiting on it: this machine's own requests would
// otherwise queue behind them for the browser's connections.
export function useRunTargetApis(alias: string | null, vaultRevision: number) {
  const apis = useMemo(() => {
    if (alias === null) return { sessions: terminalSessionsApi, workspaces: terminalWorkspacesApi, restore: terminalRestoreApi, vaultRevision, reads: null };
    const reads = createReadScope();
    return {
      sessions: runTargetTerminalSessionsApi(alias, reads),
      workspaces: createTerminalWorkspacesApi(engineTerminalBase(alias), reads),
      restore: createTerminalRestoreApi(engineTerminalBase(alias), reads),
      vaultRevision,
      reads,
    };
  }, [alias, vaultRevision]);
  useEffect(() => () => apis.reads?.end(), [apis]);
  return apis;
}
