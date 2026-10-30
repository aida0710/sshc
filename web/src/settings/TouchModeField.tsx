import { useTranslate } from "../i18n/context";
import { saveTouchMode, useDetectedTouchMode, useTouchMode, type FixedTouchMode } from "../terminal/touchMode";
import { FieldRow } from "../ui/FieldRow";
import { CheckboxField, sizedControl } from "../ui/form";

// How terminals take touch in the preset being edited, the same choice as in a terminal's "…" (which changes
// the preset this machine uses). It is one of the screen's settings, not the terminal's, so it is saved as
// soon as it changes and is not part of the page's draft and Save.
export function TouchModeField({ presetId }: { presetId: string }) {
  const t = useTranslate();
  const mode = useTouchMode(presetId);
  const detected = useDetectedTouchMode();
  return (
    <FieldRow label={t("terminal.touchModeLabel")} hint={t("terminal.touchModeSettingHint")} interactiveChildren>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <CheckboxField label={t("terminal.touchModeAutoShort")} checked={mode === "auto"}
          onChange={(checked) => saveTouchMode(checked ? "auto" : detected, presetId)} />
        {/* While detection is on, this shows what it picked and waits for detection to be turned off. */}
        <select className={sizedControl("w-[140px]")} aria-label={t("terminal.touchModeFixedLabel")}
          value={mode === "auto" ? detected : mode} disabled={mode === "auto"}
          onChange={(event) => saveTouchMode(event.target.value as FixedTouchMode, presetId)}>
          <option value="touch">{t("terminal.touchModeTouchShort")}</option>
          <option value="mouse">{t("terminal.touchModeMouseShort")}</option>
        </select>
      </div>
    </FieldRow>
  );
}
