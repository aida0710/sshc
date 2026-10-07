import { act, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { UpdateApi, UpdateStatus } from "../api/update";
import { useUpdateStatus } from "./useUpdateStatus";

afterEach(() => vi.useRealTimers());

it("retains update progress across a restart outage and stops polling after confirmation", async () => {
  vi.useFakeTimers();
  const idle: UpdateStatus = { current: "v1.0.0", available: true, canUpdate: true };
  const api: UpdateApi = {
    updateStatus: vi.fn().mockResolvedValue(idle),
    previewUpdate: vi.fn(),
    startUpdate: vi.fn(),
  };
  const { result, unmount } = renderHook(() => useUpdateStatus(api));
  await act(async () => {});
  vi.mocked(api.updateStatus).mockRejectedValue(new TypeError("restart removed listener"));
  await act(async () => result.current.setStatus({
    ...idle, job: { id: "fixture-job", target: "v1.1.0", state: "accepted", problem: "" },
  }));
  expect(result.current.status?.job?.state).toBe("accepted");
  vi.mocked(api.updateStatus).mockResolvedValue({
    ...idle, job: { id: "fixture-job", target: "v1.1.0", state: "restarting", problem: "" },
  });
  await act(async () => vi.advanceTimersByTimeAsync(2000));
  expect(result.current.status?.job?.state).toBe("restarting");
  vi.mocked(api.updateStatus).mockResolvedValue({
    current: "v1.1.0", available: false,
    job: { id: "fixture-job", target: "v1.1.0", state: "succeeded", problem: "" },
  });
  await act(async () => vi.advanceTimersByTimeAsync(2000));
  expect(result.current.status?.job?.state).toBe("succeeded");
  const confirmedCalls = vi.mocked(api.updateStatus).mock.calls.length;
  await act(async () => vi.advanceTimersByTimeAsync(10000));
  expect(api.updateStatus).toHaveBeenCalledTimes(confirmedCalls);
  unmount();
});
