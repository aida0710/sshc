import { useEffect, useState } from "react";

// A whole number typed into the transfer settings, shown in `scale` units
// (a MiB setting has a scale of 1 MiB) and committed in the setting's own
// unit. A value outside min and max, counted in the shown unit, is put back.
export function TransferIntegerSetting({ label, value, min, max, scale = 1, unit, onCommit }: {
  label: string;
  value: number;
  min: number;
  max: number;
  scale?: number;
  unit?: string;
  onCommit: (value: number) => void;
}) {
  const shownValue = value / scale;
  const [draft, setDraft] = useState(String(shownValue));
  useEffect(() => setDraft(String(shownValue)), [shownValue]);
  function commit() {
    const parsed = Number(draft);
    if (!Number.isInteger(parsed) || parsed < min || parsed > max) {
      setDraft(String(shownValue));
      return;
    }
    onCommit(parsed * scale);
  }
  return (
    <label className="flex min-w-0 flex-col gap-1.5 text-sm text-ink-muted">
      <span>{label}</span>
      <span className="flex items-center gap-2">
        <input
          type="number"
          inputMode="numeric"
          aria-label={label}
          min={min}
          max={max}
          step={1}
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          onBlur={commit}
          onKeyDown={(event) => {
            if (event.key === "Enter") event.currentTarget.blur();
            if (event.key === "Escape") {
              setDraft(String(shownValue));
              event.currentTarget.blur();
            }
          }}
          className="h-11 w-24 rounded-md border border-control-line bg-control px-3 text-right text-sm tabular-nums md:h-9 [@media(pointer:coarse)]:h-11"
        />
        {unit === undefined ? null : <span aria-hidden="true">{unit}</span>}
      </span>
    </label>
  );
}
