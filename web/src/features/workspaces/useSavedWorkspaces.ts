import { useCallback, useEffect, useState } from "react";
import { workspaceApi, type SavedWorkspace } from "./api";

// useSavedWorkspaces は、保存したレイアウトの一覧を読み、読み直す手段を渡す。
// 読めなかった一覧を「保存レイアウトが無い」と見せないように、失敗は別に持つ。
// 保存と削除のあとの読み直しでも、操作は成功しているので失敗だけを知らせる。
export function useSavedWorkspaces() {
  const [saved, setSaved] = useState<SavedWorkspace[]>([]);
  const [loadFailed, setLoadFailed] = useState(false);
  const reload = useCallback(async () => {
    try {
      setSaved(await workspaceApi.list());
      setLoadFailed(false);
    } catch {
      setLoadFailed(true);
    }
  }, []);
  useEffect(() => {
    void reload();
  }, [reload]);
  return { saved, loadFailed, reload };
}
