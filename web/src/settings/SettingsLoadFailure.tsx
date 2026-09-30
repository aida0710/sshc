import { useTranslate } from "../i18n/context";
import { PanelState } from "../ui/PanelState";
import { Button } from "../ui/surface";
import type { SettingsLoad } from "./useSettingsLoad";

// SettingsLoadFailure は、設定の節を読み込めなかったことと、読み直すボタンを出す。
// 読み込めるまで保存させない（useSettingsLoad）ので、利用者がやり直せる場所が要る。
export function SettingsLoadFailure({ settingsLoad, title }: { settingsLoad: SettingsLoad; title: string }) {
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
