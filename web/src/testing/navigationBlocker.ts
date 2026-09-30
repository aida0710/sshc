import { act } from "@testing-library/react";
import type { NavigationBlocker } from "../routing/useSectionRoute";

// captureNavigationBlocker は、画面が onNavigationBlockerChange で登録する blocker を
// 受け取り、テストから画面の移動を試せるようにする。blocker が無ければ移動は通る。
export function captureNavigationBlocker() {
  let blocker: NavigationBlocker | null = null;
  return {
    onNavigationBlockerChange: (next: NavigationBlocker | null) => {
      blocker = next;
    },
    registered: () => blocker !== null,
    // tryNavigate は、pathname と search への移動を blocker に通し、通ったかを返す。
    tryNavigate: (pathname: string, search = ""): boolean => {
      let allowed = true;
      act(() => {
        allowed = blocker?.({ pathname, search }) ?? true;
      });
      return allowed;
    },
  };
}
