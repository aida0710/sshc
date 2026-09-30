import { useCallback, useRef, useState } from "react";
import type { SyncApi, SyncBucketStatus, SyncHistory, SyncPushDraft } from "../api/sync";
import type { Translate } from "../i18n/context";
import { useRequestGeneration } from "../ui/useRequestGeneration";

export type BucketStatusState =
  | { phase: "idle" }
  | { phase: "loading" }
  | { phase: "error"; message: string }
  | { phase: "ready"; value: SyncBucketStatus };

export type HistoryState =
  | { phase: "idle" }
  | { phase: "loading" }
  | { phase: "error"; message: string }
  | { phase: "ready"; value: SyncHistory };

export function useSyncRemoteState(api: Pick<SyncApi, "syncPushDraft" | "syncBucketStatus" | "syncHistory">, t: Translate) {
  const [bucketState, setBucketState] = useState<BucketStatusState>({
    phase: "idle",
  });
  const [historyState, setHistoryState] = useState<HistoryState>({
    phase: "idle",
  });
  const [selectedHistoryKey, setSelectedHistoryKey] = useState<string | null>(
    null,
  );
  const [bucketHistoryExpanded, setBucketHistoryExpanded] = useState(false);
  const [pushDraft, setPushDraft] = useState<SyncPushDraft | null>(null);
  const [pushMessage, setPushMessage] = useState("");
  const pushMessageDirty = useRef(false);

  // The bucket is read by the polling and again after a push. Only the latest
  // read is shown, so a slow poll cannot put back the state before the push.
  const bucketGeneration = useRequestGeneration();
  const refreshBucket = useCallback(async () => {
    const isCurrent = bucketGeneration.begin();
    setBucketState({ phase: "loading" });
    try {
      const value = await api.syncBucketStatus();
      if (isCurrent()) setBucketState({ phase: "ready", value });
    } catch {
      if (isCurrent()) setBucketState({ phase: "error", message: t("sync.bucketStatusFailed") });
    }
  }, [api, bucketGeneration, t]);

  const refreshHistory = useCallback(async () => {
    setHistoryState({ phase: "loading" });
    try {
      const value = await api.syncHistory();
      setHistoryState({ phase: "ready", value });
      setSelectedHistoryKey((current) =>
        current !== null &&
        value.revisions.some((revision) => revision.key === current)
          ? current
          : null,
      );
    } catch {
      setHistoryState({ phase: "error", message: t("sync.historyFailed") });
    }
  }, [api, t]);

  const refreshPushDraft = useCallback(async () => {
    try {
      const draft = await api.syncPushDraft();
      setPushDraft(draft);
      if (!pushMessageDirty.current) setPushMessage(draft.message);
    } catch {
      setPushDraft(null);
    }
  }, [api]);

  const resetBucket = useCallback(() => {
    bucketGeneration.retire();
    setBucketState({ phase: "idle" });
  }, [bucketGeneration]);
  const resetHistory = useCallback(
    () => setHistoryState({ phase: "idle" }),
    [],
  );
  const resetPush = useCallback(() => {
    setPushDraft(null);
    setPushMessage("");
    pushMessageDirty.current = false;
  }, []);

  return {
    bucketState,
    historyState,
    selectedHistoryKey,
    bucketHistoryExpanded,
    pushDraft,
    pushMessage,
    refreshBucket,
    refreshHistory,
    refreshPushDraft,
    resetBucket,
    resetHistory,
    resetPush,
    selectHistory: (key: string) => setSelectedHistoryKey(key),
    toggleBucketHistory: () => setBucketHistoryExpanded((current) => !current),
    editPushMessage: (message: string) => {
      pushMessageDirty.current = true;
      setPushMessage(message);
    },
    acceptPushMessage: () => {
      pushMessageDirty.current = false;
    },
  };
}
