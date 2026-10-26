import type { SplitViewNode } from "./layout";
import { paneBounds } from "./geometry";

// The gap between two panes in the map, in pixels, so the panes read as separate boxes.
const paneGap = 1.5;

// SplitLayoutMap is a small picture of how a split arranges its panes. With highlight, that
// session's pane is drawn in the accent, so a row can show where its session sits.
export function SplitLayoutMap({
  root,
  highlight,
  className,
}: {
  root: SplitViewNode | null;
  highlight?: string;
  // className sets the size, e.g. "h-[18px] w-[26px]".
  className: string;
}) {
  return (
    <span aria-hidden="true" data-split-layout-map className={`relative inline-block shrink-0 rounded-[3px] bg-ink/10 ${className}`}>
      {paneBounds(root).map(({ sessionId, bounds }) => (
        <span
          key={sessionId}
          className={`absolute rounded-[2px] ${sessionId === highlight ? "bg-accent" : "bg-ink/30"}`}
          style={{
            left: `calc(${bounds.left}% + ${paneGap}px)`,
            top: `calc(${bounds.top}% + ${paneGap}px)`,
            width: `calc(${bounds.width}% - ${paneGap * 2}px)`,
            height: `calc(${bounds.height}% - ${paneGap * 2}px)`,
          }}
        />
      ))}
    </span>
  );
}
