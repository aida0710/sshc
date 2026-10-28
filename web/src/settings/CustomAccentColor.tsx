import { useEffect, useRef, useState } from "react";
import { useTranslate } from "../i18n/context";
import { contrastRatio, minimumAccentContrast, parseAccentColor } from "../theme/accentColor";
import { useTheme } from "../theme/context";
import { themeAttributes } from "../theme/theme";
import { narrowControl } from "../ui/form";
import { Notice } from "../ui/surface";

// CustomAccentColor is the row under the accent swatches while the custom accent is chosen: the system's
// colour picker and the same colour as a code to type. Dragging in the picker shows the colour at once and
// saves it when the picker is closed, so one choice is one save.
export function CustomAccentColor({ color, disabled, onPreview, onChoose }: {
  color: string;
  disabled: boolean;
  onPreview: (color: string) => void;
  onChoose: (color: string) => void;
}) {
  const t = useTranslate();
  const picker = useRef<HTMLInputElement>(null);
  const [typed, setTyped] = useState(color);
  const [typedInvalid, setTypedInvalid] = useState(false);
  const { resolved } = useTheme();
  const pageProbe = useRef<HTMLSpanElement>(null);
  const [lowContrast, setLowContrast] = useState(false);

  useEffect(() => {
    setTyped(color);
    setTypedInvalid(false);
  }, [color]);

  // React's onChange on a colour input follows every step of the drag (the input event); the change event
  // comes once, when the picker closes.
  useEffect(() => {
    const element = picker.current;
    if (element === null) return;
    const commit = () => onChoose(element.value);
    element.addEventListener("change", commit);
    return () => element.removeEventListener("change", commit);
  }, [onChoose]);

  // The colour must stand out from the page it sits on in the theme on screen (the settings board is the
  // sidebar colour). The probe carries that theme itself, so it reads the right colour in the same commit
  // that changes the theme, before ThemeProvider has put the theme on <html>.
  useEffect(() => {
    if (pageProbe.current === null) return;
    const page = parseAccentColor(getComputedStyle(pageProbe.current).getPropertyValue("--ui-sidebar"));
    setLowContrast(page !== null && contrastRatio(color, page) < minimumAccentContrast);
  }, [color, resolved]);

  function commitTyped() {
    const parsed = parseAccentColor(typed);
    if (parsed === null) {
      setTypedInvalid(true);
      return;
    }
    setTypedInvalid(false);
    if (parsed !== color) onChoose(parsed);
  }

  return (
    <div className="mt-4 flex flex-col gap-3">
      <span ref={pageProbe} {...themeAttributes(resolved)} hidden />
      <div className="flex items-center gap-2.5">
        <input
          ref={picker}
          type="color"
          aria-label={t("appearance.customColor")}
          value={color}
          disabled={disabled}
          onChange={(event) => onPreview(event.target.value)}
          className="h-8 w-11 shrink-0 cursor-pointer rounded-md border border-control-line bg-control p-1 disabled:cursor-default"
        />
        <input
          type="text"
          aria-label={t("appearance.customColorCode")}
          aria-invalid={typedInvalid}
          value={typed}
          disabled={disabled}
          spellCheck={false}
          autoCapitalize="none"
          onChange={(event) => setTyped(event.target.value)}
          onBlur={commitTyped}
          onKeyDown={(event) => {
            if (event.key === "Enter") commitTyped();
          }}
          className={`${narrowControl} font-mono`}
        />
      </div>
      {typedInvalid ? <p className="text-xs text-danger">{t("appearance.customColorInvalid")}</p> : null}
      {lowContrast ? <Notice compact>{t("appearance.customColorLowContrast")}</Notice> : null}
    </div>
  );
}
