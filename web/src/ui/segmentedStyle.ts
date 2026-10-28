// The look shared by Segmented (a choice among a few values) and SegmentedTabs (tabs inside a page):
// the options side by side in a sunken, outlined strip, the chosen one on the selection colour.
// The two differ in what they mean to screen readers, not in how they look.
export const segmentedStrip = "inline-flex gap-0.5 rounded-md border border-line bg-well p-0.5";

export function segmentClass(selected: boolean): string {
  return `h-[26px] rounded-[6px] px-2.5 text-[12.5px] transition-colors ${
    selected ? "bg-select-fill text-ink" : "text-ink-muted hover:bg-hover hover:text-ink"
  }`;
}
