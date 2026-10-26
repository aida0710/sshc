import type { MouseEvent } from "react";

// opensInThisPage says whether the app should handle a click on one of its own
// links: a plain left click that nothing else has taken. A modified or middle
// click keeps the browser's behaviour (a new tab or window).
export function opensInThisPage(event: MouseEvent<HTMLAnchorElement>): boolean {
  return !(event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey);
}
