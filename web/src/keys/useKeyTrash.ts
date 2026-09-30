import { useState } from "react";
import { useTranslate } from "../i18n/context";
import type { KeyInventoryResponse, KeyItem, KeysApi } from "./api";
import { describeBlockers } from "./labels";
import { KeyOperationRefused, type RunKeyOperation } from "./useKeyOperation";

type KeyTrashOptions = {
  api: KeysApi;
  inventory: KeyInventoryResponse | null;
  refresh: () => Promise<void>;
  runKeyOperation: RunKeyOperation;
  clearFailure: () => void;
};

// useKeyTrash は、鍵をごみ箱へ移す前の確認と、ごみ箱からの復元・完全な削除を扱う。
export function useKeyTrash({ api, inventory, refresh, runKeyOperation, clearFailure }: KeyTrashOptions) {
  const t = useTranslate();
  const [pendingTrash, setPendingTrash] = useState<KeyItem | null>(null);

  // 同じフィンガープリントの秘密鍵・公開鍵・証明書は、一緒にごみ箱へ移る。
  function trashMembers(item: KeyItem): KeyItem[] {
    if (item.fingerprint === "" || inventory === null) return [item];
    return inventory.items.filter((candidate) => candidate.fingerprint === item.fingerprint);
  }

  async function moveToTrash(keyId: string) {
    await runKeyOperation(async () => {
      await api.trash(keyId);
      setPendingTrash(null);
      await refresh();
    }, "keys.trashFailed");
  }

  async function restore(entryId: string) {
    await runKeyOperation(async () => {
      const response = await api.restore(entryId);
      if (response.blockers.length > 0) {
        throw new KeyOperationRefused("keys.restoreRefused", {
          blockers: describeBlockers(response.blockers, t),
        });
      }
      await refresh();
    }, "keys.restoreFailed");
  }

  // purge の失敗は、確認ダイアログ（KeyTrashSection）が自分の中に出すので、画面の通知には
  // 出さない。前の操作の失敗だけを消す。
  async function purge(entryId: string): Promise<boolean> {
    clearFailure();
    try {
      await api.purge(entryId);
    } catch {
      return false;
    }
    await refresh();
    return true;
  }

  return { pendingTrash, setPendingTrash, trashMembers, moveToTrash, restore, purge };
}
