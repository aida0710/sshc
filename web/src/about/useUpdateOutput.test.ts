import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { UpdateApi, UpdateJob } from "../api/update";
import { updateLog } from "../testing/updateLog";
import { useUpdateOutput } from "./useUpdateOutput";

// pollInterval is the hook's updateOutputPollMs.
const pollInterval = 1000;

function job(state: UpdateJob["state"], id = "job-one"): UpdateJob {
  return { id, target: "v1.2.0", state, problem: "" };
}

function updatesWith(readLog: UpdateApi["updateLog"]): UpdateApi {
  return { updateStatus: vi.fn(), checkUpdate: vi.fn(), previewUpdate: vi.fn(), startUpdate: vi.fn(), updateLog: readLog };
}

function renderOutput(api: UpdateApi, initial: UpdateJob | undefined) {
  return renderHook(({ current }) => useUpdateOutput(api, current), { initialProps: { current: initial } });
}

async function nextPoll() {
  await act(async () => { await vi.advanceTimersByTimeAsync(pollInterval); });
}

describe("useUpdateOutput", () => {
  beforeEach(() => { vi.useFakeTimers(); });
  afterEach(() => { vi.useRealTimers(); });

  it("shows the installer's output as it is written and resumes from the cursor after the engine restarts", async () => {
    const first = "sshc: installing sshc-linux-amd64 (v1.2.0)\n";
    const second = "sshc: stopping the engine (pid 4242)\n";
    const readLog = vi.fn()
      .mockResolvedValueOnce(updateLog(first))
      .mockRejectedValueOnce(new TypeError("Failed to fetch"))
      .mockResolvedValueOnce(updateLog(second, first.length));
    const { result } = renderOutput(updatesWith(readLog), job("installing"));
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    expect(result.current.output).toBe(first);

    await nextPoll();
    expect(result.current.output).toBe(first);
    await nextPoll();
    expect(readLog).toHaveBeenLastCalledWith(first.length);
    expect(result.current.output).toBe(first + second);
  });

  it("waits while the log still belongs to the previous job", async () => {
    const readLog = vi.fn()
      .mockResolvedValueOnce(updateLog("old job\n", 0, { jobId: "job-zero" }))
      .mockResolvedValueOnce(updateLog("new job\n"));
    const { result } = renderOutput(updatesWith(readLog), job("accepted"));
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    expect(result.current.output).toBe("");
    await nextPoll();
    expect(readLog).toHaveBeenLastCalledWith(0);
    expect(result.current.output).toBe("new job\n");
  });

  it("reads the output of an ended job to its end once, so it stays after a reload", async () => {
    const readLog = vi.fn()
      .mockResolvedValueOnce(updateLog("part one\n", 0, { cursor: { requested: 0, start: 0, next: 9, end: 18, truncated: false } }))
      .mockResolvedValueOnce(updateLog("part two\n", 9, { reachedCapacity: true }));
    const { result } = renderOutput(updatesWith(readLog), job("succeeded"));
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    await nextPoll();
    expect(result.current).toEqual({ output: "part one\npart two\n", reachedCapacity: true });
    await nextPoll();
    await nextPoll();
    expect(readLog).toHaveBeenCalledTimes(2);
  });

  it("starts over for a new job", async () => {
    const readLog = vi.fn()
      .mockResolvedValueOnce(updateLog("first job\n"))
      .mockResolvedValue(updateLog("second job\n", 0, { jobId: "job-two" }));
    const { result, rerender } = renderOutput(updatesWith(readLog), job("failed"));
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    expect(result.current.output).toBe("first job\n");

    rerender({ current: job("accepted", "job-two") });
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    expect(readLog).toHaveBeenLastCalledWith(0);
    expect(result.current.output).toBe("second job\n");
  });

  it("reads nothing while the engine reports no job", async () => {
    const readLog = vi.fn();
    renderOutput(updatesWith(readLog), undefined);
    await nextPoll();
    expect(readLog).not.toHaveBeenCalled();
  });
});
