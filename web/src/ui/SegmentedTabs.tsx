import { segmentClass, segmentedStrip } from "./segmentedStyle";
import { activateTabFromKeyboard } from "./tabKeyboard";

export type SegmentedTab<T extends string> = { value: T; label: string };

// SegmentedTabs is a second level of tabs inside a page or a panel (Settings > Terminal, the views of a
// connection's Advanced). It looks like Segmented (segmentedStyle.ts) but is a tab list: the page draws
// the panel itself, and tabId and panelId tie the two together for screen readers.
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
    <div role="tablist" aria-label={label} className={segmentedStrip}>
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
            className={segmentClass(selected)}
          >
            {tab.label}
          </button>
        );
      })}
    </div>
  );
}
