import type { ReactNode } from "react";
import { mobileViewportQuery, useMediaQuery } from "./useMediaQuery";

// PageBoard is the rounded card a page sits in, 8px inside the main pane as in the design. aside is a
// column beside the card (Settings lists its pages there). On a phone the screen is the card, so the
// page only keeps its margins.
//
// fill is for a workspace that takes the whole pane, such as Connections: the card fills the pane's
// height and width without padding, and the page lays out its own scrolling columns inside it.
export function PageBoard({
  labelledBy,
  aside,
  fill = false,
  className = "",
  children,
}: {
  // labelledBy names the region after the page's heading, when the page has one.
  labelledBy?: string;
  aside?: ReactNode;
  fill?: boolean;
  className?: string;
  children: ReactNode;
}) {
  const compact = useMediaQuery(mobileViewportQuery);
  if (compact) {
    return (
      <section aria-labelledby={labelledBy} className={`${fill ? "flex h-full min-h-0 flex-col" : "px-3 pb-[18px] pt-3.5"} ${className}`}>
        {children}
      </section>
    );
  }
  return (
    <div className={fill ? "flex h-full min-h-0 gap-2 p-2" : "mx-auto flex min-h-full w-full max-w-6xl gap-2 p-2"}>
      {aside}
      <section
        aria-labelledby={labelledBy}
        className={`@container min-w-0 flex-1 rounded-xl border border-line bg-sidebar shadow-pane ${fill ? "flex min-h-0 flex-col overflow-hidden" : "px-8 py-[26px]"} ${className}`}
      >
        {children}
      </section>
    </div>
  );
}
