import { useCallback, useEffect, useState } from "react";
import type { SyncApi, SyncStatus } from "../api/sync";
import { useTranslate } from "../i18n/context";
import { usePolling } from "../ui/usePolling";

// Another device's push shows up within half a minute; the bucket listing
// is a paid request, so the panel does not ask more often.
const bucketPollIntervalMs = 30_000;

export type SyncStatusState =
  | { phase: "loading" }
  | { phase: "error"; message: string }
  | { phase: "ready"; value: SyncStatus };

type SyncStatusOptions = {
  api: Pick<SyncApi, "syncStatus">;
  // What the screen reads about the bucket once it knows the status: the
  // bucket's state, the history once a key is set, and the next push while
  // this device may push.
  refreshBucket: () => Promise<void>;
  resetBucket: () => void;
  refreshHistory: () => Promise<void>;
  resetHistory: () => void;
  refreshPushDraft: () => Promise<void>;
  resetPush: () => void;
  clearError: () => void;
};

// useSyncStatus は、同期の状態を読み、それに応じてバケットの状態・履歴・次の送信の
// 内容を読み直す。設定済みでVaultが開いているあいだは、バケットの状態と次の送信の内容を
// 定期的に読む。自動同期が裏で送ったあと、このマシンのファイルが変わったあとも、
// 画面の下書きが古いまま残らないようにするためである。
export function useSyncStatus({
  api,
  refreshBucket,
  resetBucket,
  refreshHistory,
  resetHistory,
  refreshPushDraft,
  resetPush,
  clearError,
}: SyncStatusOptions) {
  const t = useTranslate();
  const [statusState, setStatusState] = useState<SyncStatusState>({ phase: "loading" });

  const refreshFromStatus = useCallback((next: SyncStatus) => {
    if (next.configured) void refreshBucket();
    else resetBucket();
    if (next.configured && next.keyConfigured) void refreshHistory();
    else resetHistory();
    if (next.configured && next.direction !== "pull") void refreshPushDraft();
    else resetPush();
  }, [refreshBucket, refreshHistory, refreshPushDraft, resetBucket, resetHistory, resetPush]);

  const reload = useCallback(async () => {
    setStatusState({ phase: "loading" });
    try {
      const next = await api.syncStatus();
      setStatusState({ phase: "ready", value: next });
      refreshFromStatus(next);
      if (next.direction === "pull" && next.auto.phase === "blocked" && next.auto.detail === "remote_moved") {
        clearError();
      }
    } catch {
      setStatusState({ phase: "error", message: t("sync.statusFailed") });
    }
  }, [api, refreshFromStatus, t, clearError]);

  useEffect(() => {
    void reload();
  }, [reload]);

  const status = statusState.phase === "ready" ? statusState.value : null;
  const mayPush = status !== null && status.direction !== "pull";
  usePolling(() => Promise.all([refreshBucket(), mayPush ? refreshPushDraft() : undefined]), {
    intervalMs: bucketPollIntervalMs,
    enabled: status !== null && status.configured && !status.locked,
  });

  // adopt は、操作の応答に入っていた状態を、読み直さずにそのまま使う。
  const adopt = useCallback((value: SyncStatus) => setStatusState({ phase: "ready", value }), []);

  // adoptAndRefresh は、バケットかこのマシンの変更が変わる操作（送信・同期・最初の設定）の
  // 応答を使い、その状態に応じて読み直す。reload と違い、画面を読み込み中に戻さない。
  const adoptAndRefresh = useCallback((value: SyncStatus) => {
    adopt(value);
    refreshFromStatus(value);
  }, [adopt, refreshFromStatus]);

  return { statusState, reload, adopt, adoptAndRefresh };
}
