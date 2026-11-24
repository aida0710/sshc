import { useTranslate } from "../i18n/context";
import { parseAccentColor } from "../theme/accentColor";
import { remoteSessionColorOf, withRemoteSessionColor } from "../theme/remoteSessionColor";
import { FieldRow } from "../ui/FieldRow";
import { Notice, Segmented } from "../ui/surface";
import { useAsyncOperation } from "../ui/useAsyncOperation";
import { ColorCodeInput } from "./ColorCodeInput";
import { updatePreset, useSettings } from "./settingsStore";

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
  const preset = useSettings().document?.presets.find((candidate) => candidate.id === presetId) ?? { id: presetId };
  const color = remoteSessionColorOf(preset);
  const chosen: RemoteSessionColorChoice = color === null ? "accent" : "custom";
  const saving = useAsyncOperation();
  const describeFailure = { describe: () => t("appearance.remoteSessionColorSaveFailed") };

  function save(next: string | null) {
    void saving.run(() => updatePreset(presetId, (current) => withRemoteSessionColor(current, next)), describeFailure);
  }

  function choose(next: RemoteSessionColorChoice) {
    if (next === chosen) return;
    save(next === "accent" ? null : remoteColorOnScreen());
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
            disabled={saving.busy}
            labels={{
              picker: t("appearance.remoteSessionColor.picker"),
              code: t("appearance.remoteSessionColor.code"),
              lowContrast: t("appearance.remoteSessionColorLowContrast"),
            }}
            // 枠は画面の一番外側を囲むので、アプリの地の色と見分けられないと困る。
            contrastAgainst="--ui-canvas"
            // 枠はリモートセッションを選んでいる間だけ出るので、ここでは動かしている途中の色を見せない。
            onPreview={() => undefined}
            onChoose={save}
          />
        )}
        {saving.error === "" ? null : <Notice tone="danger">{saving.error}</Notice>}
      </div>
    </FieldRow>
  );
}
