import { useTranslate } from "../i18n/context";
import { useDetectedTouchMode, withTouchMode, type FixedTouchMode, type TouchMode } from "../terminal/touchMode";
import { FieldRow } from "../ui/FieldRow";
import { CheckboxField, sizedControl } from "../ui/form";
import { presetInterfaceOf } from "./presetInterface";
import { usePresetInterfaceEdit, useSettingsDraft } from "./settingsDraftContext";

// How terminals take touch in the preset being edited, the same choice as in a terminal's "…" (which changes the
// preset this machine uses, at once), and whether a touch terminal underlines its links.
export function TouchModeField({ presetId }: { presetId: string }) {
  const t = useTranslate();
  const settings = presetInterfaceOf(useSettingsDraft().document, presetId);
  const editInterface = usePresetInterfaceEdit(presetId);
  const mode: TouchMode = settings.touchMode ?? "auto";
  const detected = useDetectedTouchMode();
  const choose = (next: TouchMode) => editInterface("touchMode", (current) => withTouchMode(current, next));
  return (
    <FieldRow label={t("terminal.touchModeLabel")} interactiveChildren>
      <div className="flex flex-col gap-3">
        <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
          <CheckboxField label={t("terminal.touchModeAutoShort")} checked={mode === "auto"}
            onChange={(checked) => choose(checked ? "auto" : detected)} />
          {/* While detection is on, this shows what it picked and waits for detection to be turned off. */}
          <select className={sizedControl("w-[140px]")} aria-label={t("terminal.touchModeFixedLabel")}
            value={mode === "auto" ? detected : mode} disabled={mode === "auto"}
            onChange={(event) => choose(event.target.value as FixedTouchMode)}>
            <option value="touch">{t("terminal.touchModeTouchShort")}</option>
            <option value="mouse">{t("terminal.touchModeMouseShort")}</option>
          </select>
        </div>
        <CheckboxField label={t("terminal.touchLinkUnderlinesLabel")} hint={t("terminal.touchLinkUnderlinesHint")}
          checked={settings.hideTouchLinkUnderlines !== true}
          onChange={(checked) => editInterface("hideTouchLinkUnderlines", (current) => ({ ...current, hideTouchLinkUnderlines: !checked }))} />
      </div>
    </FieldRow>
  );
}
