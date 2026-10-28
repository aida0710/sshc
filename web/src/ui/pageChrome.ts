import { createContext, type ReactNode } from "react";

// PageChrome is what the app's frame lends to a page's heading. On a wide screen the heading shows the
// inspector's toggle first among its actions, so the toggle sits in the page instead of floating over the
// corner of its card. On a phone the heading's name is hidden (the top bar already says it), so the heading
// puts its actions in the top bar's slot rather than on a row of their own. A page that has gone one step in
// (a connection opened from the list) puts its way back in the top bar's back slot, in place of the menu.
export type PageChrome = {
  inspectorToggle: ReactNode;
  phoneActionsSlot: HTMLElement | null;
  phoneBackSlot: HTMLElement | null;
};

export const PageChromeContext = createContext<PageChrome>({ inspectorToggle: null, phoneActionsSlot: null, phoneBackSlot: null });
