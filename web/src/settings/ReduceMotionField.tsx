import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { reduceMotionChoiceOf, reduceMotionChoices, withReduceMotion, type ReduceMotionChoice } from "../theme/motionContext";
import { FieldRow } from "../ui/FieldRow";
import { Notice, Segmented } from "../ui/surface";
import { useAsyncOperation } from "../ui/useAsyncOperation";
import { updatePreset, useSettings } from "./settingsStore";

const choiceLabelKeys: Record<ReduceMotionChoice, MessageKey> = {
  system: "appearance.reduceMotion.system",
  on: "appearance.reduceMotion.on",
  off: "appearance.reduceMotion.off",
};

// Whether the preset being edited leaves animation out. MotionProvider reads the preset this machine uses, so
// a change to it shows at once.
export function ReduceMotionField({ presetId }: { presetId: string }) {
  const t = useTranslate();
  const preset = useSettings().document?.presets.find((candidate) => candidate.id === presetId) ?? { id: presetId };
  const chosen = reduceMotionChoiceOf(preset);
  const saving = useAsyncOperation();

  function choose(next: ReduceMotionChoice) {
    if (next === chosen) return;
    void saving.run(
      () => updatePreset(presetId, (current) => withReduceMotion(current, next)),
      { describe: () => t("appearance.reduceMotionSaveFailed") },
    );
  }

  return (
    <FieldRow label={t("appearance.reduceMotion")} hint={t("appearance.reduceMotionHint")} interactiveChildren>
      {/* The choices take their own width, as the theme and accent choices do, not the row's. */}
      <div className="flex flex-col items-start gap-3">
        <Segmented
          label={t("appearance.reduceMotion")}
          value={chosen}
          options={reduceMotionChoices.map((choice) => ({ value: choice, label: t(choiceLabelKeys[choice]) }))}
          onChange={choose}
        />
        {saving.error === "" ? null : <Notice tone="danger">{saving.error}</Notice>}
      </div>
    </FieldRow>
  );
}
