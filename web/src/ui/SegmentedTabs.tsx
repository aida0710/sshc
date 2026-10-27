import { activateTabFromKeyboard } from "./tabKeyboard";

export type SegmentedTab<T extends string> = { value: T; label: string };

// SegmentedTabs is a second level of tabs inside a page or a panel (Settings > Terminal, the views of a
// connection's Advanced): the names side by side in a sunken strip, the chosen one raised on the card's
// colour. The page draws the panel itself; tabId and panelId tie the two together for screen readers.
export function SegmentedTabs<T extends string>({
  label,
  tabs,
  value,
  onChange,
  tabId,
  panelId,
}: {
  label: string;
  tabs: readonly SegmentedTab<T>[];
  value: T;
  onChange: (value: T) => void;
  tabId: (value: T) => string;
  panelId: string;
}) {
  const values = tabs.map((tab) => tab.value);
  return (
    <div role="tablist" aria-label={label} className="inline-flex gap-0.5 rounded-[9px] bg-well p-[3px]">
      {tabs.map((tab, index) => {
        const selected = tab.value === value;
        return (
          <button
            key={tab.value}
            id={tabId(tab.value)}
            type="button"
            role="tab"
            aria-selected={selected}
            aria-controls={panelId}
            tabIndex={selected ? 0 : -1}
            onClick={() => onChange(tab.value)}
            onKeyDown={(event) => activateTabFromKeyboard(event, index, values, onChange)}
            className={`h-7 rounded-[7px] px-3.5 text-[13px] transition-colors ${
              selected ? "bg-sidebar text-ink shadow-[0_1px_2px_rgba(0,0,0,0.25)]" : "text-ink-muted hover:text-ink"
            }`}
          >
            {tab.label}
          </button>
        );
      })}
    </div>
  );
}
