import { useCallback, useEffect, useState } from "react";
import { useTranslate } from "../i18n/context";
import type { KeyInventoryResponse, KeysApi, KeyVariant, TrashListResponse } from "./api";

export type KeyInventoryState = "loading" | "ready" | "error";

// useKeyInventory は、~/.ssh の鍵の一覧、ごみ箱、作れる鍵の種類を読む。
export function useKeyInventory({ api, fail }: { api: KeysApi; fail: (message: string) => void }) {
  const t = useTranslate();
  const [state, setState] = useState<KeyInventoryState>("loading");
  const [inventory, setInventory] = useState<KeyInventoryResponse | null>(null);
  const [trash, setTrash] = useState<TrashListResponse | null>(null);
  const [variants, setVariants] = useState<KeyVariant[]>([]);

  const read = useCallback(async () => {
    const [nextInventory, nextTrash, nextAlgorithms] = await Promise.all([
      api.inventory(),
      api.listTrash(),
      api.algorithms(),
    ]);
    setInventory(nextInventory);
    setTrash(nextTrash);
    setVariants(nextAlgorithms.variants);
  }, [api]);

  // 初回の読み込みだけは、失敗したら画面全体を失敗にして、もう一度試せるようにする。
  const load = useCallback(async () => {
    setState("loading");
    try {
      await read();
      setState("ready");
    } catch {
      setState("error");
    }
  }, [read]);

  // 操作のあとの読み直しは、失敗しても読めている一覧を残し、失敗だけを知らせる。
  // 操作そのものは成功しているので、画面全体を失敗に置き換えない。
  const refresh = useCallback(async () => {
    try {
      await read();
    } catch {
      fail(t("keys.refreshFailed"));
    }
  }, [fail, read, t]);

  useEffect(() => {
    void load();
  }, [load]);

  return { state, inventory, trash, variants, load, refresh };
}
