import { useCallback, useState } from "react";
import type { BrowserLocation, NavigateLocationOptions, NavigationBlocker } from "./useSectionRoute";
import { useUnsavedDraftGuard } from "./useUnsavedDraftGuard";

// useDraftDiscardConfirmation は、未保存の下書きを捨てる操作（画面の移動、別の項目を
// 開くなど）の前に確かめる。呼び手は confirming のあいだ DiscardDraftDialog を出し、
// confirmDiscard と keepEditing をそのボタンに渡す。
//
// discard は下書きを保存済みの内容に戻す。先に戻してから操作を進めるので、同じ画面の
// 中の移動でも、移動先で再び確かめない。操作で部品ごと消える下書きなら渡さなくてよい。
//
// navigationKeepsDraft は、下書きを残したまま進めてよい移動先で true を返す。移動先が
// 同じ画面で、画面が自分で確かめるときに使う。描画ごとに作り直さない関数を渡す
// （useUnsavedDraftGuard の blocker と同じ理由）。
export function useDraftDiscardConfirmation({
  dirty,
  discard,
  navigationKeepsDraft,
  onNavigationBlockerChange,
  onNavigateLocation,
}: {
  dirty: boolean;
  discard?: (() => void) | undefined;
  navigationKeepsDraft?: ((next: BrowserLocation) => boolean) | undefined;
  onNavigationBlockerChange?: ((blocker: NavigationBlocker | null) => void) | undefined;
  onNavigateLocation?: ((url: string, options?: NavigateLocationOptions) => void) | undefined;
}) {
  const [pendingAction, setPendingAction] = useState<(() => void) | null>(null);

  const leaveBlocker = useCallback<NavigationBlocker>((next) => {
    if (navigationKeepsDraft?.(next) === true) return true;
    setPendingAction(() => () => {
      onNavigationBlockerChange?.(null);
      onNavigateLocation?.(`${next.pathname}${next.search}`);
    });
    return false;
  }, [navigationKeepsDraft, onNavigateLocation, onNavigationBlockerChange]);
  useUnsavedDraftGuard({ dirty, blocker: leaveBlocker, onNavigationBlockerChange });

  // 下書きがあれば action の前に確かめ、無ければそのまま行う。
  const confirmBefore = useCallback((action: () => void) => {
    if (dirty) setPendingAction(() => action);
    else action();
  }, [dirty]);

  const confirmDiscard = useCallback(() => {
    const action = pendingAction;
    setPendingAction(null);
    discard?.();
    action?.();
  }, [discard, pendingAction]);

  const keepEditing = useCallback(() => setPendingAction(null), []);

  return { confirming: pendingAction !== null, confirmBefore, confirmDiscard, keepEditing };
}
