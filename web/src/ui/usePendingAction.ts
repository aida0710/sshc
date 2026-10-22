import { useCallback, useRef, useState } from "react";

// usePendingAction は、画面のいくつかのボタンのうち、どれで始めた操作を待っているかを持つ。
// 待っている間は running がその操作を指すので、押したボタンだけを busy にし、ほかのボタンを
// 止めるのに使う。一度に一つだけ走らせ、待っている間に始めた二つ目は走らせない（同じファイルや
// 設定を続けて書き換えると、二つ目が古い内容を前提にして断られるため）。失敗はそのまま返すので、
// 理由を出すのは呼び手である。
export function usePendingAction<Action extends string>() {
  const [running, setRunning] = useState<Action | null>(null);
  // 同じ描画の中で続けて押されても二つ目を走らせないよう、描画を待たない印も持つ。
  const runningNow = useRef<Action | null>(null);
  const start = useCallback(async (action: Action, work: () => Promise<unknown>): Promise<void> => {
    if (runningNow.current !== null) return;
    runningNow.current = action;
    setRunning(action);
    try {
      await work();
    } finally {
      runningNow.current = null;
      setRunning(null);
    }
  }, []);
  return { running, start };
}
