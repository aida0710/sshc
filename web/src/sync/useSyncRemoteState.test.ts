import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { SyncBucketStatus } from "../api/sync";
import { useSyncRemoteState } from "./useSyncRemoteState";

function bucketCheckedAt(checkedAt: string): SyncBucketStatus {
  return { checkedAt, localIsLive: true, history: [], historyTruncated: false };
}

describe("useSyncRemoteState", () => {
  it("shows the latest bucket read even when an earlier read answers after it", async () => {
    let answerEarlier: (status: SyncBucketStatus) => void = () => undefined;
    const syncBucketStatus = vi.fn()
      .mockImplementationOnce(() => new Promise<SyncBucketStatus>((resolve) => { answerEarlier = resolve; }))
      .mockResolvedValueOnce(bucketCheckedAt("after-push"));
    const api = { syncBucketStatus, syncHistory: vi.fn(), syncPushDraft: vi.fn() };
    const { result } = renderHook(() => useSyncRemoteState(api, (key) => key));

    let earlier: Promise<void> = Promise.resolve();
    act(() => {
      earlier = result.current.refreshBucket();
    });
    await act(() => result.current.refreshBucket());
    await act(async () => {
      answerEarlier(bucketCheckedAt("before-push"));
      await earlier;
    });

    expect(result.current.bucketState).toEqual({ phase: "ready", value: bucketCheckedAt("after-push") });
  });
});
