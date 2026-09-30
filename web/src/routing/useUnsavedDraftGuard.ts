import { useEffect } from "react";
import { useBeforeUnloadWarning } from "../ui/useBeforeUnloadWarning";
import type { NavigationBlocker } from "./useSectionRoute";

// useUnsavedDraftGuard は、未保存の下書きがある間、画面内の移動を blocker に通し、
// ブラウザのリロードとタブを閉じる操作の前にブラウザの確認を出させる。
//
// blocker は移動先を受け取り、そのまま進めてよいときは true を返す。止めるときは
// false を返し、破棄を確かめるダイアログは呼び手が出す。blocker は描画ごとに作り直さない
// （useCallback で包む）。作り直すと登録し直しが毎回起きる。
export function useUnsavedDraftGuard({
  dirty,
  blocker,
  onNavigationBlockerChange,
}: {
  dirty: boolean;
  blocker: NavigationBlocker;
  onNavigationBlockerChange?: ((blocker: NavigationBlocker | null) => void) | undefined;
}) {
  useEffect(() => {
    if (!dirty) {
      onNavigationBlockerChange?.(null);
      return;
    }
    onNavigationBlockerChange?.(blocker);
    return () => onNavigationBlockerChange?.(null);
  }, [blocker, dirty, onNavigationBlockerChange]);
  useBeforeUnloadWarning(dirty);
}
