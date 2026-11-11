import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import type { KeyBarLayout, KeyBarRow, KeySlot } from "./keyBarLayout";
import { KeyFace, spokenKeyName } from "./KeyFace";
import { keyBarKeyGap } from "./keyBarStyles";

const keyShape = `h-11 min-w-0 truncate rounded-inset font-mono text-2xs ${keyBarKeyGap}`;
// The row wraps instead of scrolling as on the bar: to arrange the keys, all of them must be in sight and 44px wide.
const keyGrid = "grid grid-cols-[repeat(auto-fill,minmax(2.75rem,1fr))] gap-y-1";

// KeyBarPreview shows a layout in the key bar's order, with the keys behind 「…」 under it. A tap selects a key (a second
// tap lets go of it) for the buttons beside it to move. The keys sit on the canvas, as the terminal's key bar does: a
// light theme's card is as white as its sections and its terminal, which would leave only the keys' names.
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
    <div data-key-bar-preview className="flex flex-col gap-2 rounded-inset bg-canvas p-2">
      <div role="group" aria-label={t("keyBar.keysRow")} className={keyGrid}>
        {row("keys")}
        {layout.moreKeys.length === 0 ? null : (
          <span aria-hidden="true" className={`${keyShape} flex items-center justify-center bg-card text-ink-faint`}>
            <Icon name="moreHorizontal" className="size-4" />
          </span>
        )}
      </div>
      <span className="px-0.5 text-xs text-ink-faint">{t("keyBar.moreRow")}</span>
      <div role="group" aria-label={t("keyBar.moreRow")} className={keyGrid}>
        {layout.moreKeys.length === 0 ? <span className="col-span-full px-0.5 text-xs text-ink-faint">{t("keyBar.moreRowEmpty")}</span> : row("moreKeys")}
      </div>
    </div>
  );
}
