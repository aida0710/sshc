import { useLayoutEffect } from "react";

// useReportDirty は、下書きに未保存の変更があるかを親へ伝える。
//
// useEffect ではなく useLayoutEffect で伝える。useEffect は、保存のバーを消した描画を
// 画面に出したあとで動くので、タブ → HostDetailPanel → ページと伝わって editorDirty が
// false になるまでに数ミリ秒の間ができ、そのあいだに別の接続を選ぶと破棄の確認が出る。
// useLayoutEffect の中の更新は、画面に出す前に同期して描画し終える。
export function useReportDirty(dirty: boolean, onDirtyChange: ((dirty: boolean) => void) | undefined) {
  useLayoutEffect(() => onDirtyChange?.(dirty), [dirty, onDirtyChange]);
}
