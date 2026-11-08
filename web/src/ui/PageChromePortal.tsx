import { useLayoutEffect, useRef, type ReactNode } from "react";
import { createPortal } from "react-dom";

// PageChromePortal puts a part of a page (its actions, its way back) in a slot of the app's top bar on a phone
// (see pageChrome), shown only while the page is. React hides a section kept behind the one shown (KeptSections),
// but not what the section put elsewhere through a portal. React takes a hidden section's layout effects down in
// the same update that hides it, so the holder hides itself then, and shows itself again when they come back.
export function PageChromePortal({ slot, children }: { slot: HTMLElement; children: ReactNode }) {
  const holder = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const element = holder.current;
    if (element === null) return;
    element.style.display = "contents";
    return () => {
      element.style.display = "none";
    };
  }, []);
  return createPortal(
    <div ref={holder} data-page-chrome style={{ display: "none" }}>
      {children}
    </div>,
    slot,
  );
}
