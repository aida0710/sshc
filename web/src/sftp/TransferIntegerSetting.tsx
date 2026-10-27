import { useEffect, useState } from "react";
import type { DescribableControlProps } from "../ui/describeControl";
import { sizedControl } from "../ui/form";
import { isImeComposing } from "../ui/imeComposition";

// A whole number typed into the transfer settings, shown in `scale` units
// (a MiB setting has a scale of 1 MiB) and committed in the setting's own
// unit. A value outside min and max, counted in the shown unit, is put back.
//
// It is one input with an optional unit after it, so a form row can show one
// alone or several inside a sentence. label names the input for screen readers;
// the row shows the visible name. A row's description is passed on to the input.
export function TransferIntegerSetting({ label, value, min, max, scale = 1, unit, widthClass, onCommit, ...description }: {
  label: string;
  value: number;
  min: number;
  max: number;
  scale?: number;
  unit?: string;
  // widthClass is the input's width, such as "w-[80px]", written out here for Tailwind to find.
  widthClass: string;
  onCommit: (value: number) => void;
} & DescribableControlProps) {
  const shownValue = value / scale;
  const [draft, setDraft] = useState(String(shownValue));
  useEffect(() => setDraft(String(shownValue)), [shownValue]);
  function commit() {
    const parsed = Number(draft);
    if (!Number.isInteger(parsed) || parsed < min || parsed > max) {
      setDraft(String(shownValue));
      return;
    }
    // Leaving the field without changing it is not a change, and must not
    // be reported as one that was saved.
    if (parsed === shownValue) return;
    onCommit(parsed * scale);
  }
  return (
    <span className="inline-flex items-center gap-2">
      <input
        type="number"
        inputMode="numeric"
        aria-label={label}
        {...description}
        min={min}
        max={max}
        step={1}
        value={draft}
        onChange={(event) => setDraft(event.target.value)}
        onBlur={commit}
        onKeyDown={(event) => {
          if (isImeComposing(event)) return;
          if (event.key === "Enter") event.currentTarget.blur();
          if (event.key === "Escape") {
            setDraft(String(shownValue));
            event.currentTarget.blur();
          }
        }}
        className={`${sizedControl(widthClass)} font-mono tabular-nums`}
      />
      {unit === undefined ? null : <span aria-hidden="true" className="text-[12.5px] text-ink-muted">{unit}</span>}
    </span>
  );
}
