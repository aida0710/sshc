import { useCallback, useEffect, useState } from "react";
import { ApiError } from "../api/client";
import type { RemoteEnginesApi, RemoteEngineStatus } from "../api/remoteEngines";
import type { Translate } from "../i18n/context";
import { runTargetProblemText } from "./runTargetProblem";
import { useEngineAction, type EngineActionState } from "./useEngineAction";

export type EngineInfoState = EngineActionState & {
  status: RemoteEngineStatus | null;
  // problem says why the status could not be read. The actions stay offered:
  // an incompatible engine is the one that needs updating.
  problem: string;
  // remoteVersion is the version of an engine refused as incompatible.
  remoteVersion: string | null;
};

// useEngineInfo reads the state of a run-target engine and stops, restarts or
// updates it with the host's own sshc command (see useEngineAction).
export function useEngineInfo({
  alias,
  api,
  translate,
  vaultRevision,
}: {
  alias: string;
  api: RemoteEnginesApi;
  translate: Translate;
  // vaultRevision changes when a run-target Vault is unlocked, which is when
  // the status is read again.
  vaultRevision: number;
}): EngineInfoState {
  const [status, setStatus] = useState<RemoteEngineStatus | null>(null);
  const [problem, setProblem] = useState("");
  const [remoteVersion, setRemoteVersion] = useState<string | null>(null);

  const readStatus = useCallback(async () => {
    try {
      setStatus(await api.status(alias));
      setProblem("");
      setRemoteVersion(null);
    } catch (error) {
      setStatus(null);
      const incompatible = error instanceof ApiError && error.code === "remote_engine_incompatible";
      setRemoteVersion(incompatible ? error.problem?.detail ?? "" : null);
      setProblem(incompatible ? "" : runTargetProblemText(translate, error, alias));
    }
  }, [alias, api, translate]);

  useEffect(() => { void readStatus(); }, [readStatus, vaultRevision]);

  // The status is read again once an action ends, as it changes the version
  // or stops the engine.
  const engineAction = useEngineAction({ alias, api, translate, onEnded: () => void readStatus() });

  return { status, problem, remoteVersion, ...engineAction };
}
