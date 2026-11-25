import { useEffect, useMemo, useRef } from "react";
import { createReadScope, type ReadScope } from "../api/readScope";
import { engineTerminalBase } from "../api/remoteEngines";
import { createTerminalRestoreApi, terminalRestoreApi } from "../api/terminalRestore";
import { runTargetTerminalSessionsApi, terminalSessionsApi, type TerminalSessionsApi } from "../api/terminalSessions";
import { createTerminalWorkspacesApi, terminalWorkspacesApi } from "../api/terminalWorkspaces";

type RunTargetApis = {
  sessions: TerminalSessionsApi;
  workspaces: ReturnType<typeof createTerminalWorkspacesApi>;
  restore: ReturnType<typeof createTerminalRestoreApi>;
  vaultRevision: number;
  reads: ReadScope | null;
};

// useRunTargetApis gives the terminal API of the run target: this machine's own, or the one this machine's engine
// forwards for an engine on an SSH host. A new API is a new engine to the hooks that use it, which drop what they
// showed and read it anew; that is wanted once a run target's Vault is unlocked, so the API is built again for each
// unlock (vaultRevision). Leaving a run target ends the reads still waiting on it: this machine's own requests would
// otherwise queue behind them for the browser's connections.
//
// Coming back to a run target gives the API it had: its terminals stay mounted while another run target is shown
// (TerminalScreen), and a terminal handed another API would start over.
export function useRunTargetApis(alias: string | null, vaultRevision: number): RunTargetApis {
  const built = useRef(new Map<string, RunTargetApis>());
  const apis = useMemo(() => {
    const key = `${alias ?? ""}\n${vaultRevision}`;
    const known = built.current.get(key);
    if (known !== undefined) return known;
    const made = alias === null ? thisMachineApis(vaultRevision) : runTargetApis(alias, vaultRevision);
    built.current.set(key, made);
    return made;
  }, [alias, vaultRevision]);
  useEffect(() => () => apis.reads?.end(), [apis]);
  return apis;
}

function thisMachineApis(vaultRevision: number): RunTargetApis {
  return { sessions: terminalSessionsApi, workspaces: terminalWorkspacesApi, restore: terminalRestoreApi, vaultRevision, reads: null };
}

function runTargetApis(alias: string, vaultRevision: number): RunTargetApis {
  const reads = createReadScope();
  return {
    sessions: runTargetTerminalSessionsApi(alias, reads),
    workspaces: createTerminalWorkspacesApi(engineTerminalBase(alias), reads),
    restore: createTerminalRestoreApi(engineTerminalBase(alias), reads),
    vaultRevision,
    reads,
  };
}
