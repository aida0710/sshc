import type { CSSProperties } from "react";

// The parts of a workspace page (PageBoard fill), as Connections lays them out: the heading row, then a
// list beside what is open from it, divided by a line. On a phone the list and the open item take turns
// on the screen, so each page says which one is showing with detailOpen.

export const workspaceHeaderLayout = "shrink-0 px-3 py-3 md:px-[26px] md:pb-[22px] md:pt-5";

// The list column is --list-width wide, set by the page (Connections lets it be dragged wider).
export function workspaceColumnsLayout(compact: boolean): string {
  return `grid min-h-0 flex-1 grid-cols-1 grid-rows-[minmax(0,1fr)] border-t border-line ${
    compact ? "" : "md:grid-cols-[minmax(16rem,var(--list-width))_minmax(0,1fr)]"
  }`;
}

// fixedListWidthStyle is the list column of a page whose list is not resized, 280px as in the design.
export const fixedListWidthStyle = { "--list-width": "280px" } as CSSProperties;

export function workspaceListPane(compact: boolean, detailOpen: boolean): string {
  return `relative min-h-0 flex-col ${compact ? "" : "border-r border-line md:flex"} ${detailOpen ? "hidden" : "flex"}`;
}

// The open item scrolls on its own. On a phone the bottom bar floats over its end, so the end is padded to
// scroll clear of the bar, and scrolling a field into view stops above it.
export function workspaceDetailPane(compact: boolean, detailOpen: boolean): string {
  return `min-h-0 flex-col overflow-y-auto p-4 pb-[calc(1rem+var(--mobile-navigation-inset))] scroll-pb-(--mobile-navigation-inset) md:px-[26px] md:pt-[22px] md:pb-[calc(22px+var(--mobile-navigation-inset))] ${
    compact ? "" : "md:flex"
  } ${detailOpen ? "flex" : "hidden"}`;
}
