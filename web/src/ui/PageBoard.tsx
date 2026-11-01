import type { ReactNode } from "react";
import { mobileViewportQuery, useMediaQuery } from "./useMediaQuery";

// PageBoard is the rounded card a page sits in, 8px inside the main pane as in the design. aside is a
// column beside the card (Settings lists its pages there). On a phone the screen is the card, so the
// page only keeps its margins.
//
// fill is for a workspace that takes the whole pane, such as Connections: the card fills the pane's
// height and width without padding, and the page lays out its own scrolling columns inside it.
//
// Other pages keep a readable width (max-w-6xl). wide drops it for a page of cards that adds columns as the
// pane grows (Home). A page with an aside sits at the left beside the sidebar, so its list of pages does not
// float in the middle of a wide screen.
export function PageBoard({
  labelledBy,
  aside,
  fill = false,
  wide = false,
  className = "",
  children,
}: {
  // labelledBy names the region after the page's heading, when the page has one.
  labelledBy?: string;
  aside?: ReactNode;
  fill?: boolean;
  wide?: boolean;
  className?: string;
  children: ReactNode;
}) {
  const compact = useMediaQuery(mobileViewportQuery);
  // Both widths keep the same elements, so crossing the phone width (turning a tablet, opening developer
  // tools) does not mount the page afresh and drop what was being typed.
  const frame = compact
    ? fill ? "flex h-full min-h-0 flex-col" : ""
    : fill ? "flex h-full min-h-0 gap-2 p-2" : `flex min-h-full w-full gap-2 p-2 ${wide ? "" : aside ? "max-w-6xl" : "mx-auto max-w-6xl"}`;
  const card = compact
    ? fill ? "flex min-h-0 flex-1 flex-col" : "px-3 pb-[18px] pt-3.5"
    : `@container min-w-0 flex-1 rounded-xl border border-line bg-sidebar shadow-pane ${fill ? "flex min-h-0 flex-col overflow-hidden" : "px-8 py-[26px]"}`;
  return (
    <div className={frame}>
      {compact ? null : aside}
      <section aria-labelledby={labelledBy} className={`${card} ${className}`}>
        {children}
      </section>
    </div>
  );
}
