import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import type { KeyBarLayout, KeyBarRow, KeySlot } from "./keyBarLayout";
import { KeyFace, spokenKeyName } from "./KeyFace";
import { keyBarRowColumns } from "./keyBarStyles";

const keyShape = "h-10 min-w-0 truncate rounded-[9px] px-0.5 font-mono text-[12px]";

// KeyBarPreview shows a layout as the key bar shows it, with the keys behind 「…」 under it. A tap selects a key
// (a second tap lets go of it) for the buttons beside it to move.
export function KeyBarPreview({ layout, selected, onSelect, disabled }: {
  layout: KeyBarLayout;
  selected: KeySlot | null;
  onSelect: (slot: KeySlot | null) => void;
  disabled: boolean;
}) {
  const t = useTranslate();
  const row = (name: KeyBarRow) => layout[name].map((key, index) => {
    const isSelected = selected?.row === name && selected.index === index;
    return (
      <button key={`${name}-${index}`} type="button" data-touch-compact disabled={disabled} aria-pressed={isSelected}
        aria-label={spokenKeyName(t, key)}
        onClick={() => onSelect(isSelected ? null : { row: name, index })}
        className={`${keyShape} ${isSelected ? "bg-select-fill text-ink ring-2 ring-inset ring-accent" : "bg-card text-ink"}`}>
        <KeyFace barKey={key} iconClassName="size-[15px]" />
      </button>
    );
  });
  return (
    <div data-key-bar-preview className="flex flex-col gap-2 rounded-[10px] bg-term-bg p-2">
      <div role="group" aria-label={t("keyBar.keysRow")} className={`${keyBarRowColumns} gap-1`}>
        {row("keys")}
        {layout.moreKeys.length === 0 ? null : (
          <span aria-hidden="true" className={`${keyShape} flex items-center justify-center bg-card text-ink-faint`}>
            <Icon name="moreHorizontal" className="size-4" />
          </span>
        )}
      </div>
      <span className="px-0.5 text-[11.5px] text-ink-faint">{t("keyBar.moreRow")}</span>
      <div role="group" aria-label={t("keyBar.moreRow")} className="grid grid-cols-6 gap-1">
        {layout.moreKeys.length === 0 ? <span className="col-span-6 px-0.5 text-xs text-ink-faint">{t("keyBar.moreRowEmpty")}</span> : row("moreKeys")}
      </div>
    </div>
  );
}
