import { useTranslate } from "../i18n/context";
import { saveTouchLinkUnderlines, useTouchLinkUnderlines } from "../terminal/touchLinkUnderlines";
import { saveTouchMode, useDetectedTouchMode, useTouchMode, type FixedTouchMode } from "../terminal/touchMode";
import { FieldRow } from "../ui/FieldRow";
import { CheckboxField, sizedControl } from "../ui/form";

// How terminals take touch in the preset being edited, the same choice as in a terminal's "…" (which changes
// the preset this machine uses), and whether a touch terminal underlines its links. They are the screen's
// settings, not the terminal's, so each is saved as soon as it changes.
export function TouchModeField({ presetId }: { presetId: string }) {
  const t = useTranslate();
  const mode = useTouchMode(presetId);
  const detected = useDetectedTouchMode();
  const linksUnderlined = useTouchLinkUnderlines(presetId);
  return (
    <FieldRow label={t("terminal.touchModeLabel")} interactiveChildren>
      <div className="flex flex-col gap-3">
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
        <CheckboxField label={t("terminal.touchLinkUnderlinesLabel")} hint={t("terminal.touchLinkUnderlinesHint")}
          checked={linksUnderlined} onChange={(checked) => saveTouchLinkUnderlines(checked, presetId)} />
      </div>
    </FieldRow>
  );
}
