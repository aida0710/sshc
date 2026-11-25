import { useEffect, useRef, useState } from "react";
import { useTranslate } from "../i18n/context";
import { contrastRatio, minimumAccentContrast, parseAccentColor } from "../theme/accentColor";
import { useTheme } from "../theme/context";
import { themeAttributes } from "../theme/theme";
import { useFieldError } from "../ui/fieldError";
import { narrowControl } from "../ui/form";
import { asciiTextEntry } from "../ui/softKeyboard";
import { Notice } from "../ui/surface";
import { useTypedSetting, type ParsedSetting } from "./useTypedSetting";

// ColorCodeInput は、自分で決める色を選ぶ行: システムのカラーピッカーと、同じ色をコードで打つ欄。さし色と、
// リモートセッションの枠の色が使う。
export function ColorCodeInput({ color, labels, contrastAgainst, onChange }: {
  color: string;
  // labels は、ピッカーと打つ欄の名前と、背景と見分けにくい色のときに出す注意。
  labels: { picker: string; code: string; lowContrast: string };
  // contrastAgainst は、この色が見分けられないと困る背景の色の CSS 変数（--ui-sidebar など）。
  contrastAgainst: string;
  onChange: (color: string) => void;
}) {
  const t = useTranslate();
  const typed = useTypedSetting<string>({
    text: color,
    parse: (text): ParsedSetting<string> => {
      const parsed = parseAccentColor(text);
      return parsed === null ? { problem: t("appearance.customColorInvalid") } : { value: parsed };
    },
    onChange,
  });
  const described = useFieldError(typed.problem, {});
  const { resolved } = useTheme();
  const pageProbe = useRef<HTMLSpanElement>(null);
  const [lowContrast, setLowContrast] = useState(false);

  // The colour must stand out from the page it sits on in the theme on screen. The probe carries that theme
  // itself, so it reads the right colour in the same commit that changes the theme, before ThemeProvider has put
  // the theme on <html>.
  useEffect(() => {
    if (pageProbe.current === null) return;
    const page = parseAccentColor(getComputedStyle(pageProbe.current).getPropertyValue(contrastAgainst));
    setLowContrast(page !== null && contrastRatio(color, page) < minimumAccentContrast);
  }, [color, resolved, contrastAgainst]);

  return (
    <div className="mt-4 flex flex-col gap-3">
      <span ref={pageProbe} {...themeAttributes(resolved)} hidden />
      <div className="flex items-center gap-2.5">
        <input
          type="color"
          aria-label={labels.picker}
          value={color}
          onChange={(event) => onChange(event.target.value)}
          className="h-8 w-11 shrink-0 cursor-pointer rounded-md border border-control-line bg-control p-1 mobile-layout:h-11"
        />
        <input {...asciiTextEntry}
          type="text"
          aria-label={labels.code}
          {...described}
          value={typed.shown}
          onChange={(event) => typed.type(event.target.value)}
          onBlur={typed.leave}
          className={`${narrowControl} font-mono`}
        />
      </div>
      {lowContrast ? <Notice compact>{labels.lowContrast}</Notice> : null}
    </div>
  );
}
