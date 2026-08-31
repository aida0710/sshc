import { useCallback, useRef, useState } from "react";
import type {
  IntegrationsApi,
  SyncBucketStatus,
  SyncHistory,
  SyncPushDraft,
} from "../api/integrations";
import type { Translate } from "../i18n/context";

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

export function useSyncRemoteState(api: IntegrationsApi, t: Translate) {
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

  const refreshBucket = useCallback(async () => {
    setBucketState({ phase: "loading" });
    try {
      setBucketState({ phase: "ready", value: await api.syncBucketStatus() });
    } catch {
      setBucketState({ phase: "error", message: t("sync.bucketStatusFailed") });
    }
  }, [api, t]);

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

  const resetBucket = useCallback(() => setBucketState({ phase: "idle" }), []);
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
