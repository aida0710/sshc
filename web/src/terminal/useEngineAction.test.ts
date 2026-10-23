import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import type { RemoteEngineActionLog, RemoteEnginesApi } from "../api/remoteEngines";
import { engineActionLog } from "../testing/engineActionLog";
import { useEngineAction } from "./useEngineAction";

const translate = ((key: string) => key) as never;

// pollInterval is the hook's engineActionPollMs.
const pollInterval = 1000;

function enginesApi(actionLog: RemoteEnginesApi["actionLog"], control: RemoteEnginesApi["control"] = vi.fn(async () => ({ generation: 1 }))): RemoteEnginesApi {
  return {
    list: vi.fn(), add: vi.fn(), remove: vi.fn(), open: vi.fn(), status: vi.fn(), unlockVault: vi.fn(),
    control, actionLog,
  };
}

// runningLog is a log of an action that has written text so far and not ended.
function runningLog(text: string, from: number): RemoteEngineActionLog {
  return engineActionLog({
    done: false, succeeded: false, output: text,
    cursor: { requested: from, start: from, next: from + text.length, end: from + text.length, truncated: false },
  });
}

function renderAction(api: RemoteEnginesApi) {
  const onEnded = vi.fn();
  const hook = renderHook(() => useEngineAction({ alias: "r540", api, translate, onEnded }));
  return { ...hook, onEnded };
}

async function nextPoll() {
  await act(async () => { await vi.advanceTimersByTimeAsync(pollInterval); });
}

describe("useEngineAction", () => {
  beforeEach(() => { vi.useFakeTimers(); });
  afterEach(() => { vi.useRealTimers(); });

  it("shows each part of the host's output as it is read and ends once the log says so", async () => {
    const first = "sshc: updating v0.51.0 to v0.54.0\n";
    const last = "sshc: updated to v0.54.0\n";
    const actionLog = vi.fn()
      .mockResolvedValueOnce(runningLog(first, 0))
      .mockResolvedValueOnce(engineActionLog({ output: last, cursor: { requested: first.length, start: first.length, next: first.length + last.length, end: first.length + last.length, truncated: false } }));
    const { result, onEnded } = renderAction(enginesApi(actionLog));

    await act(async () => { await result.current.run("update"); });
    expect(result.current.output).toBe(first);
    expect(result.current.running).toBe("update");
    expect(result.current.lastResult).toBeNull();

    await nextPoll();
    expect(actionLog).toHaveBeenLastCalledWith("r540", first.length);
    expect(result.current.output).toBe(first + last);
    expect(result.current.running).toBeNull();
    expect(result.current.lastResult).toEqual({ action: "update", succeeded: true });
    expect(onEnded).toHaveBeenCalledTimes(1);

    await nextPoll();
    expect(actionLog).toHaveBeenCalledTimes(2);
  });

  it("keeps reading an ended action until the output before its end has been shown", async () => {
    const actionLog = vi.fn()
      .mockResolvedValueOnce(engineActionLog({ output: "a", cursor: { requested: 0, start: 0, next: 1, end: 2, truncated: false } }))
      .mockResolvedValueOnce(engineActionLog({ output: "b", cursor: { requested: 1, start: 1, next: 2, end: 2, truncated: false } }));
    const { result } = renderAction(enginesApi(actionLog));
    await act(async () => { await result.current.run("update"); });
    expect(result.current.running).toBe("update");
    await nextPoll();
    expect(result.current.output).toBe("ab");
    expect(result.current.lastResult?.succeeded).toBe(true);
  });

  it("says that the host's sshc failed when it exits with an error", async () => {
    const actionLog = vi.fn(async () => engineActionLog({ succeeded: false, output: "sshc: update failed\n" }));
    const { result } = renderAction(enginesApi(actionLog));
    await act(async () => { await result.current.run("update"); });
    expect(result.current.lastResult).toEqual({ action: "update", succeeded: false });
    expect(result.current.actionProblem).toBe("");
  });

  it("says only why an action ended without the host's sshc running to the end", async () => {
    const actionLog = vi.fn(async () => engineActionLog({ succeeded: false, problem: "remote_engine_not_installed" }));
    const { result } = renderAction(enginesApi(actionLog));
    await act(async () => { await result.current.run("update"); });
    expect(result.current.lastResult).toBeNull();
    expect(result.current.running).toBeNull();
    expect(result.current.actionProblem).toBe("runTarget.notInstalled");
  });

  it("says another action is running when the engine refuses to start one, and follows nothing", async () => {
    const actionLog = vi.fn();
    const control = vi.fn().mockRejectedValue(new ApiError("remote_engine_action_running", 409, null));
    const { result } = renderAction(enginesApi(actionLog, control));
    await act(async () => { await result.current.run("restart"); });
    expect(result.current.actionProblem).toBe("runTarget.actionRunning");
    expect(result.current.running).toBeNull();
    await nextPoll();
    expect(actionLog).not.toHaveBeenCalled();
  });

  it("retries a read that got no answer but stops when the engine no longer has the log", async () => {
    const actionLog = vi.fn()
      .mockRejectedValueOnce(new TypeError("Failed to fetch"))
      .mockRejectedValueOnce(new ApiError("remote_engine_no_action", 404, null));
    const { result } = renderAction(enginesApi(actionLog));
    await act(async () => { await result.current.run("update"); });
    expect(result.current.running).toBe("update");
    await nextPoll();
    expect(result.current.running).toBeNull();
    expect(result.current.actionProblem).toBe("runTarget.actionLogLost");
  });

  it("stops following when a later action replaced the one it started", async () => {
    const actionLog = vi.fn(async () => engineActionLog({ generation: 2 }));
    const { result } = renderAction(enginesApi(actionLog));
    await act(async () => { await result.current.run("update"); });
    expect(result.current.lastResult).toBeNull();
    expect(result.current.running).toBeNull();
    expect(result.current.actionProblem).toBe("runTarget.actionLogLost");
  });
});
