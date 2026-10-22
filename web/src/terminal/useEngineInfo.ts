import { useCallback, useEffect, useState } from "react";
import type { RemoteEngineAction, RemoteEngineActionResult, RemoteEnginesApi, RemoteEngineStatus } from "../api/remoteEngines";
import type { Translate } from "../i18n/context";
import { runTargetProblemText } from "./runTargetProblem";

export type EngineInfoState = {
  status: RemoteEngineStatus | null;
  // problem says why the status could not be read. The actions stay offered:
  // an engine whose version no longer matches is the one that needs updating.
  problem: string;
  running: RemoteEngineAction | null;
  // lastResult is what the host's sshc wrote for the last action.
  lastResult: (RemoteEngineActionResult & { action: RemoteEngineAction }) | null;
  actionProblem: string;
  run: (action: RemoteEngineAction) => Promise<void>;
};

// useEngineInfo reads the state of a run-target engine and stops, restarts or
// updates it with the host's own sshc command.
export function useEngineInfo({
  alias,
  api,
  translate,
  localVersion,
  vaultRevision,
}: {
  alias: string;
  api: RemoteEnginesApi;
  translate: Translate;
  localVersion: string;
  // vaultRevision changes when a run-target Vault is unlocked, which is when
  // the status is read again.
  vaultRevision: number;
}): EngineInfoState {
  const [status, setStatus] = useState<RemoteEngineStatus | null>(null);
  const [problem, setProblem] = useState("");
  const [running, setRunning] = useState<RemoteEngineAction | null>(null);
  const [lastResult, setLastResult] = useState<EngineInfoState["lastResult"]>(null);
  const [actionProblem, setActionProblem] = useState("");

  const readStatus = useCallback(async () => {
    try {
      setStatus(await api.status(alias));
      setProblem("");
    } catch (error) {
      setStatus(null);
      setProblem(runTargetProblemText(translate, error, alias, localVersion));
    }
  }, [alias, api, localVersion, translate]);

  useEffect(() => { void readStatus(); }, [readStatus, vaultRevision]);

  const run = useCallback(async (action: RemoteEngineAction) => {
    setRunning(action);
    setActionProblem("");
    setLastResult(null);
    try {
      setLastResult({ ...(await api.control(alias, action)), action });
    } catch (error) {
      setActionProblem(runTargetProblemText(translate, error, alias, localVersion));
    } finally {
      setRunning(null);
    }
    await readStatus();
  }, [alias, api, localVersion, readStatus, translate]);

  return { status, problem, running, lastResult, actionProblem, run };
}
