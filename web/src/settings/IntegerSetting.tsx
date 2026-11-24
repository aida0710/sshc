import { useTranslate } from "../i18n/context";
import type { DescribableControlProps } from "../ui/describeControl";
import { sizedControl } from "../ui/form";
import { useFieldError } from "../ui/fieldError";
import { useTypedSetting, type ParsedSetting } from "./useTypedSetting";

// A whole number typed into a settings page, shown in `scale` units (a MiB setting has a scale of 1 MiB) and kept in
// the draft in the setting's own unit. A value outside min and max, counted in the shown unit, is kept as typed and
// said to be wrong under the row, and nothing can be saved until it is put right.
//
// It is one input with an optional unit after it, so a form row can show one alone or several inside a sentence. label
// names the input for screen readers; the row shows the visible name. A row's description is passed on to the input.
export function IntegerSetting({ label, value, min, max, scale = 1, unit, widthClass, onChange, ...description }: {
  label: string;
  value: number;
  min: number;
  max: number;
  scale?: number;
  unit?: string;
  // widthClass is the input's width, such as "w-[80px]", written out here for Tailwind to find.
  widthClass: string;
  onChange: (value: number) => void;
} & DescribableControlProps) {
  const t = useTranslate();
  const typed = useTypedSetting<number>({
    text: String(value / scale),
    parse: (text): ParsedSetting<number> => {
      const parsed = Number(text);
      return text.trim() === "" || !Number.isInteger(parsed) || parsed < min || parsed > max
        ? { problem: t("settings.integerOutOfRange", { name: label, min, max }) }
        : { value: parsed * scale };
    },
    onChange,
  });
  const described = useFieldError(typed.problem, description);
  return (
    <span className="inline-flex items-center gap-2">
      <input
        type="number"
        inputMode="numeric"
        aria-label={label}
        {...described}
        min={min}
        max={max}
        step={1}
        value={typed.shown}
        onChange={(event) => typed.type(event.target.value)}
        onBlur={typed.leave}
        className={`${sizedControl(widthClass)} font-mono tabular-nums`}
      />
      {unit === undefined ? null : <span aria-hidden="true" className="text-xs text-ink-muted">{unit}</span>}
    </span>
  );
}
