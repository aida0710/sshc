import type { ReactNode } from "react";
import { mobileViewportQuery, useMediaQuery } from "./useMediaQuery";

// PageBoard is the rounded card a page sits in, 8px inside the main pane as in the design. aside is a
// column beside the card (Settings lists its pages there). On a phone the screen is the card, so the
// page only keeps its margins.
export function PageBoard({
  labelledBy,
  aside,
  className = "",
  children,
}: {
  // labelledBy names the region after the page's heading, when the page has one.
  labelledBy?: string;
  aside?: ReactNode;
  className?: string;
  children: ReactNode;
}) {
  const compact = useMediaQuery(mobileViewportQuery);
  if (compact) {
    return (
      <section aria-labelledby={labelledBy} className={`px-3 pb-[18px] pt-3.5 ${className}`}>
        {children}
      </section>
    );
  }
  return (
    <div className="mx-auto flex min-h-full w-full max-w-6xl gap-2 p-2">
      {aside}
      <section
        aria-labelledby={labelledBy}
        className={`@container min-w-0 flex-1 rounded-xl border border-line bg-sidebar px-8 py-[26px] shadow-pane ${className}`}
      >
        {children}
      </section>
    </div>
  );
}
