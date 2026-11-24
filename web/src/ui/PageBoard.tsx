import type { ReactNode } from "react";
import { saveBarSlotClass, SaveBarSlotContext, useSaveBarSlot } from "./saveBarSlot";
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
//
// stretch makes the card a column at least as tall as the pane, on a phone too, so that a page with little to
// show can put it in the middle of the space left.
//
// saveBar gives the page a place at the bottom of the card for the bar that saves or discards its changes
// (DraftSaveBar). The bar stays at the bottom of the screen while a long page scrolls.
export function PageBoard({
  labelledBy,
  aside,
  fill = false,
  wide = false,
  stretch = false,
  saveBar = false,
  className = "",
  children,
}: {
  // labelledBy names the region after the page's heading, when the page has one.
  labelledBy?: string;
  aside?: ReactNode;
  fill?: boolean;
  wide?: boolean;
  stretch?: boolean;
  saveBar?: boolean;
  className?: string;
  children: ReactNode;
}) {
  const compact = useMediaQuery(mobileViewportQuery);
  const { slot, slotRef } = useSaveBarSlot();
  const column = stretch || saveBar;
  // Both widths keep the same elements, so crossing the phone width (turning a tablet, opening developer
  // tools) does not mount the page afresh and drop what was being typed.
  const frame = compact
    ? fill ? "flex h-full min-h-0 flex-col" : column ? "flex min-h-full flex-col" : ""
    : fill ? "flex h-full min-h-0 gap-2 p-2" : `flex min-h-full w-full gap-2 p-2 ${wide ? "" : aside ? "max-w-6xl" : "mx-auto max-w-6xl"}`;
  const card = compact
    ? fill ? "flex min-h-0 flex-1 flex-col" : `px-3 pb-[18px] pt-3.5 [--save-bar-end:18px] [--save-bar-x:0.75rem] ${column ? "flex flex-1 flex-col" : ""}`
    : `@container min-w-0 flex-1 rounded-lg border border-line bg-sidebar shadow-pane ${fill ? "flex min-h-0 flex-col overflow-hidden" : `px-8 py-[26px] [--save-bar-end:26px] [--save-bar-x:2rem] ${column ? "flex flex-col" : ""}`}`;
  return (
    <div className={frame}>
      {compact ? null : aside}
      <section aria-labelledby={labelledBy} className={`${card} ${className}`}>
        {saveBar ? (
          <SaveBarSlotContext.Provider value={slot}>
            {/* 中身は1つの塊にして、帯だけを板の下の端へ押しやる。 */}
            <div className={`min-w-0 ${stretch ? "flex flex-1 flex-col" : "shrink-0"}`}>{children}</div>
            <div ref={slotRef} className={saveBarSlotClass} />
          </SaveBarSlotContext.Provider>
        ) : children}
      </section>
    </div>
  );
}
