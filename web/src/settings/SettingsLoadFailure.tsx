import { useTranslate } from "../i18n/context";
import { PanelState } from "../ui/PanelState";
import { Button } from "../ui/surface";
import type { Load } from "../ui/useLoad";

// SettingsLoadFailure は、設定の節を読み込めなかったことと、読み直すボタンを出す。
//
// 設定の保存は節全体を置き換えるので、読み込めないまま既定値の画面で保存すると、保存済みの設定が
// すべて既定値に戻る。節は読み込めるまで（useLoad が "loaded" になるまで）保存させないので、
// 利用者がやり直せる場所が要る。
export function SettingsLoadFailure({ settingsLoad, title }: { settingsLoad: Load<unknown>; title: string }) {
  const t = useTranslate();
  if (settingsLoad.state !== "failed") return null;
  return (
    <PanelState
      tone="failed"
      title={title}
      className="mb-5"
      action={<Button onClick={settingsLoad.retry}>{t("shell.bootstrapRetry")}</Button>}
    />
  );
}
