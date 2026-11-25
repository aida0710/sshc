import { useTranslate } from "../i18n/context";
import { parseAccentColor } from "../theme/accentColor";
import { remoteSessionColorOf, withRemoteSessionColor } from "../theme/remoteSessionColor";
import { FieldRow } from "../ui/FieldRow";
import { Segmented } from "../ui/surface";
import { ColorCodeInput } from "./ColorCodeInput";
import { useSettingsDraft } from "./settingsDraftContext";

type RemoteSessionColorChoice = "accent" | "custom";

// さし色から自分で決める色に替えるときは、いま画面にある枠の色（さし色）から始める。替えただけでは見た目が
// 変わらず、色を動かして初めて変わる。スタイルシートが無いとき（テスト）は、明るいテーマの薄紫から始める。
function remoteColorOnScreen(): string {
  return parseAccentColor(getComputedStyle(document.documentElement).getPropertyValue("--ui-remote")) ?? "#6550d0"; // palette-exempt
}

// RemoteSessionColorField は、編集しているプリセットの、リモートセッションの枠と名札の色。接続先に色を付けた
// sshc エンジンでは、そちらを使う。
export function RemoteSessionColorField({ presetId }: { presetId: string }) {
  const t = useTranslate();
  const { document, editPreset } = useSettingsDraft();
  const preset = document?.presets.find((candidate) => candidate.id === presetId) ?? { id: presetId };
  const color = remoteSessionColorOf(preset);
  const chosen: RemoteSessionColorChoice = color === null ? "accent" : "custom";

  function change(next: string | null) {
    editPreset(presetId, "appearance.remoteSessionColor", (current) => withRemoteSessionColor(current, next));
  }

  function choose(next: RemoteSessionColorChoice) {
    if (next === chosen) return;
    change(next === "accent" ? null : remoteColorOnScreen());
  }

  return (
    <FieldRow label={t("appearance.remoteSessionColor")} hint={t("appearance.remoteSessionColorHint")} interactiveChildren>
      <div className="flex flex-col items-start gap-3">
        <Segmented
          label={t("appearance.remoteSessionColor")}
          value={chosen}
          options={[
            { value: "accent", label: t("appearance.remoteSessionColor.accent") },
            { value: "custom", label: t("appearance.remoteSessionColor.custom") },
          ]}
          onChange={choose}
        />
        {color === null ? null : (
          <ColorCodeInput
            color={color}
            labels={{
              picker: t("appearance.remoteSessionColor.picker"),
              code: t("appearance.remoteSessionColor.code"),
              lowContrast: t("appearance.remoteSessionColorLowContrast"),
            }}
            // 枠は画面の一番外側を囲むので、アプリの地の色と見分けられないと困る。
            contrastAgainst="--ui-canvas"
            onChange={change}
          />
        )}
      </div>
    </FieldRow>
  );
}
