import { useCallback, useEffect, useState } from "react";

export type SettingsLoadState = "loading" | "loaded" | "failed";
export type SettingsLoad = { state: SettingsLoadState; retry: () => void };

// useSettingsLoad は、設定の節を読み込み、読み込めたかを返す。
//
// 設定の保存は節全体を置き換えるので、読み込めないまま既定値の画面で保存すると、
// 保存済みの設定がすべて既定値に戻る。呼び手は "loaded" になるまで保存させない。
// load は読み込んだ値を下書きへ移すところまで行い、useCallback で固定して渡す。
export function useSettingsLoad(load: () => Promise<void>): SettingsLoad {
  const [state, setState] = useState<SettingsLoadState>("loading");
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let active = true;
    load().then(
      () => { if (active) setState("loaded"); },
      () => { if (active) setState("failed"); },
    );
    return () => { active = false; };
  }, [load, attempt]);

  const retry = useCallback(() => {
    setState("loading");
    setAttempt((current) => current + 1);
  }, []);

  return { state, retry };
}
