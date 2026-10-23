import { useCallback, useRef, useState } from "react";
import { ApiError } from "../api/client";
import type { RemoteEngineAction, RemoteEngineActionLog, RemoteEnginesApi } from "../api/remoteEngines";
import type { Translate } from "../i18n/context";
import { usePolling } from "../ui/usePolling";
import { runTargetFailureText, runTargetProblemText } from "./runTargetProblem";

// engineActionPollMs is how often a running action's output is read. The
// host's sshc writes a line every few seconds while it downloads and installs,
// so once a second shows each line as it comes without asking more often.
const engineActionPollMs = 1000;

// maxEngineActionOutput is how much of an action's output stays on screen, the
// same 64 KiB the engine keeps for each action.
const maxEngineActionOutput = 64 * 1024;

export type EngineActionResult = { action: RemoteEngineAction; succeeded: boolean };

export type EngineActionState = {
  running: RemoteEngineAction | null;
  // output is what the host's sshc has written for the running or last action,
  // growing while it runs.
  output: string;
  // lastResult says how the host's sshc exited; null while an action runs or
  // when it ended for another reason, which actionProblem says.
  lastResult: EngineActionResult | null;
  actionProblem: string;
  run: (action: RemoteEngineAction) => Promise<void>;
};

// FollowedAction is the action this screen started and follows in the log.
type FollowedAction = { action: RemoteEngineAction; generation: number };

// useEngineAction starts stopping, restarting or updating a run-target engine
// and follows the host's sshc output until it ends. The action runs on this
// machine's engine, so leaving the screen stops only the following.
export function useEngineAction({
  alias,
  api,
  translate,
  onEnded,
}: {
  alias: string;
  api: RemoteEnginesApi;
  translate: Translate;
  // onEnded runs once an action has ended, however it ended.
  onEnded: () => void;
}): EngineActionState {
  const [running, setRunning] = useState<RemoteEngineAction | null>(null);
  const [followed, setFollowed] = useState<FollowedAction | null>(null);
  const [output, setOutput] = useState("");
  const [lastResult, setLastResult] = useState<EngineActionResult | null>(null);
  const [actionProblem, setActionProblem] = useState("");
  const cursor = useRef(0);

  const end = useCallback((result: EngineActionResult | null, problem: string) => {
    setFollowed(null);
    setRunning(null);
    setLastResult(result);
    setActionProblem(problem);
    onEnded();
  }, [onEnded]);

  const run = useCallback(async (action: RemoteEngineAction) => {
    setRunning(action);
    setActionProblem("");
    setLastResult(null);
    setOutput("");
    cursor.current = 0;
    try {
      const started = await api.control(alias, action);
      setFollowed({ action, generation: started.generation });
    } catch (error) {
      setRunning(null);
      setActionProblem(runTargetProblemText(translate, error, alias));
    }
  }, [alias, api, translate]);

  usePolling(async (isCurrent) => {
    if (followed === null) return;
    let log: RemoteEngineActionLog;
    try {
      log = await api.actionLog(alias, cursor.current);
    } catch (error) {
      // A request that never got an answer is retried on the next tick; an
      // answer refusing it means the progress is gone.
      if (!isCurrent() || !(error instanceof ApiError)) return;
      end(null, runTargetProblemText(translate, error, alias));
      return;
    }
    if (!isCurrent()) return;
    if (log.generation !== followed.generation) {
      // Another screen started a later action, so this one's output is gone.
      end(null, runTargetFailureText(translate, "remote_engine_no_action", alias));
      return;
    }
    cursor.current = log.cursor.next;
    if (log.output !== "") setOutput((shown) => keepNewest(shown + log.output));
    // The end is read only once every byte before it has been shown.
    if (!log.done || log.cursor.next < log.cursor.end) return;
    if (log.problem !== undefined) {
      end(null, runTargetFailureText(translate, log.problem, alias));
      return;
    }
    end({ action: followed.action, succeeded: log.succeeded }, "");
  }, { intervalMs: engineActionPollMs, enabled: followed !== null, immediately: true });

  return { running, output, lastResult, actionProblem, run };
}

function keepNewest(output: string): string {
  return output.length > maxEngineActionOutput ? output.slice(output.length - maxEngineActionOutput) : output;
}
