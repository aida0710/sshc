import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { SyncBucketStatus, SyncPushDraft } from "../api/sync";
import { useSyncRemoteState } from "./useSyncRemoteState";

function bucketCheckedAt(checkedAt: string): SyncBucketStatus {
  return { checkedAt, localIsLive: true, history: [], historyTruncated: false };
}

function draftWithMessage(message: string): SyncPushDraft {
  return { message, added: 0, modified: 0, removed: 0 };
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

  it("shows the latest draft even when an earlier read answers after it", async () => {
    let answerEarlier: (draft: SyncPushDraft) => void = () => undefined;
    const syncPushDraft = vi.fn()
      .mockImplementationOnce(() => new Promise<SyncPushDraft>((resolve) => { answerEarlier = resolve; }))
      .mockResolvedValueOnce(draftWithMessage("after the push"));
    const api = { syncBucketStatus: vi.fn(), syncHistory: vi.fn(), syncPushDraft };
    const { result } = renderHook(() => useSyncRemoteState(api, (key) => key));

    let earlier: Promise<void> = Promise.resolve();
    act(() => {
      earlier = result.current.refreshPushDraft();
    });
    await act(() => result.current.refreshPushDraft());
    await act(async () => {
      answerEarlier(draftWithMessage("before the push"));
      await earlier;
    });

    expect(result.current.pushMessage).toBe("after the push");
    expect(result.current.pushDraft).toEqual(draftWithMessage("after the push"));
  });

  it("keeps a message typed by hand when a new draft arrives, until the message is accepted", async () => {
    const syncPushDraft = vi.fn()
      .mockResolvedValueOnce(draftWithMessage("first draft"))
      .mockResolvedValueOnce(draftWithMessage("second draft"))
      .mockResolvedValueOnce(draftWithMessage("third draft"));
    const api = { syncBucketStatus: vi.fn(), syncHistory: vi.fn(), syncPushDraft };
    const { result } = renderHook(() => useSyncRemoteState(api, (key) => key));

    await act(() => result.current.refreshPushDraft());
    expect(result.current.pushMessage).toBe("first draft");

    act(() => result.current.editPushMessage("typed by hand"));
    await act(() => result.current.refreshPushDraft());
    expect(result.current.pushMessage).toBe("typed by hand");
    expect(result.current.pushDraft).toEqual(draftWithMessage("second draft"));

    act(() => result.current.acceptPushMessage());
    await act(() => result.current.refreshPushDraft());
    expect(result.current.pushMessage).toBe("third draft");
  });
});
