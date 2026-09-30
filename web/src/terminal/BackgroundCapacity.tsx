import { useTranslate } from "../i18n/context";
import { control } from "../ui/form";
import { formatBytes } from "../ui/format";
import { Button } from "../ui/surface";

type BackgroundCapacityProps = {
  usedBytes: number;
  capacityBytes: number;
  // The limit being typed, in MiB. It takes effect only when it is saved.
  capacityInput: string;
  onCapacityInputChange: (next: string) => void;
  canSave: boolean;
  onSave: () => void;
};

// How much of the background library's limit the saved images use, and the
// field that changes the limit.
export function BackgroundCapacity({ usedBytes, capacityBytes, capacityInput, onCapacityInputChange, canSave, onSave }: BackgroundCapacityProps) {
  const t = useTranslate();
  const usedPercent = Math.min(100, (usedBytes / Math.max(capacityBytes, 1)) * 100);
  return (
    <section className="rounded-md bg-surface-subtle p-3" aria-label={t("terminal.backgroundCapacityHeading")}>
      <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
        <div className="min-w-0 flex-1">
          <div className="flex items-center justify-between gap-3 text-xs text-ink-muted">
            <span>{t("terminal.backgroundCapacityUsage", { used: formatBytes(usedBytes), capacity: formatBytes(capacityBytes) })}</span>
            <span>{Math.round(usedPercent)}%</span>
          </div>
          <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-control">
            <div className="h-full rounded-full bg-accent" style={{ width: `${usedPercent}%` }} />
          </div>
        </div>
        <label className="flex items-end gap-2">
          <span className="flex flex-col gap-1 text-xs text-ink-muted">
            {t("terminal.backgroundCapacityLabel")}
            <span className="flex items-center gap-1">
              <input type="number" min={1} max={1024} value={capacityInput} onChange={(event) => onCapacityInputChange(event.target.value)} className={`${control} w-24`} />
              <span>MiB</span>
            </span>
          </span>
          <Button disabled={!canSave} onClick={onSave}>{t("terminal.backgroundCapacitySave")}</Button>
        </label>
      </div>
      <p className="mt-2 text-xs text-ink-faint">{t("terminal.backgroundCapacityHint")}</p>
    </section>
  );
}
