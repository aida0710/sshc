import { useCallback, useState } from "react";

// usePendingResult は、押した操作が Promise を返したとき、それが終わるまで pending を true にする。
// ダイアログが「閉じずに待つ」ときに、ボタンを busy にしてダイアログを閉じる操作を止めるのに使う。
// 失敗の文は呼び手が別に渡すので、ここでは結果を捨てる。Promise を返さない操作は待たない。
export function usePendingResult() {
  const [pending, setPending] = useState(false);
  const watch = useCallback((result: void | Promise<unknown>) => {
    if (result === undefined) return;
    setPending(true);
    void result.catch(() => undefined).finally(() => setPending(false));
  }, []);
  return { pending, watch };
}
