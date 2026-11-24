import { useEffect, useRef, useState } from "react";
import { useTranslate } from "../i18n/context";
import { contrastRatio, minimumAccentContrast, parseAccentColor } from "../theme/accentColor";
import { useTheme } from "../theme/context";
import { themeAttributes } from "../theme/theme";
import { narrowControl } from "../ui/form";
import { asciiTextEntry } from "../ui/softKeyboard";
import { Notice } from "../ui/surface";

// ColorCodeInput は、自分で決める色を選ぶ行: システムのカラーピッカーと、同じ色をコードで打つ欄。さし色と、
// リモートセッションの枠の色が使う。ピッカーで動かしている間は onPreview で見せ、閉じたときに onChoose で
// 保存するので、1回の選択が1回の保存になる。
export function ColorCodeInput({ color, disabled, labels, contrastAgainst, onPreview, onChoose }: {
  color: string;
  disabled: boolean;
  // labels は、ピッカーと打つ欄の名前と、背景と見分けにくい色のときに出す注意。
  labels: { picker: string; code: string; lowContrast: string };
  // contrastAgainst は、この色が見分けられないと困る背景の色の CSS 変数（--ui-sidebar など）。
  contrastAgainst: string;
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

  // The colour must stand out from the page it sits on in the theme on screen. The probe carries that theme
  // itself, so it reads the right colour in the same commit that changes the theme, before ThemeProvider has put
  // the theme on <html>.
  useEffect(() => {
    if (pageProbe.current === null) return;
    const page = parseAccentColor(getComputedStyle(pageProbe.current).getPropertyValue(contrastAgainst));
    setLowContrast(page !== null && contrastRatio(color, page) < minimumAccentContrast);
  }, [color, resolved, contrastAgainst]);

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
          aria-label={labels.picker}
          value={color}
          disabled={disabled}
          onChange={(event) => onPreview(event.target.value)}
          className="h-8 w-11 shrink-0 cursor-pointer rounded-md border border-control-line bg-control p-1 disabled:cursor-default mobile-layout:h-11"
        />
        <input {...asciiTextEntry}
          type="text"
          aria-label={labels.code}
          aria-invalid={typedInvalid}
          value={typed}
          disabled={disabled}
          onChange={(event) => setTyped(event.target.value)}
          onBlur={commitTyped}
          onKeyDown={(event) => {
            if (event.key === "Enter") commitTyped();
          }}
          className={`${narrowControl} font-mono`}
        />
      </div>
      {typedInvalid ? <p className="text-xs text-danger">{t("appearance.customColorInvalid")}</p> : null}
      {lowContrast ? <Notice compact>{labels.lowContrast}</Notice> : null}
    </div>
  );
}
