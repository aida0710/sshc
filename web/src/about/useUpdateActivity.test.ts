import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { UpdateJob, UpdateStatus } from "../api/update";
import { checkStartedActivity } from "./updateActivity";
import { useUpdateActivity } from "./useUpdateActivity";

const idle: UpdateStatus = { current: "v1.0.0", latest: "v1.2.0", available: true };

function withJob(state: UpdateJob["state"], problem = ""): UpdateStatus {
  return { ...idle, job: { id: "job", target: "v1.2.0", state, problem } };
}

describe("useUpdateActivity", () => {
  it("adds one line per state of an update, however often the same state is polled", () => {
    const { result, rerender } = renderHook(({ status }) => useUpdateActivity(status), { initialProps: { status: idle } });
    for (const status of [withJob("installing"), withJob("installing"), withJob("restarting"), withJob("restarting")]) {
      rerender({ status });
    }
    expect(result.current.entries.map((entry) => entry.message)).toEqual(["update.installing", "update.restarting"]);
  });

  it("reports a failed update in the danger tone with its reason", () => {
    const { result, rerender } = renderHook(({ status }) => useUpdateActivity(status), { initialProps: { status: withJob("installing") } });
    rerender({ status: withJob("failed", "update_homebrew_not_updated") });
    expect(result.current.entries.map(({ message, tone }) => ({ message, tone }))).toEqual([
      { message: "update.installing", tone: "progress" },
      { message: "update.homebrewNotUpdated", tone: "danger" },
    ]);
  });

  it("keeps only the latest lines", () => {
    const { result } = renderHook(() => useUpdateActivity(null));
    act(() => {
      for (let check = 0; check < 30; check++) result.current.record(checkStartedActivity);
    });
    expect(result.current.entries).toHaveLength(20);
    expect(result.current.entries.at(-1)?.id).toBe(29);
  });
});
