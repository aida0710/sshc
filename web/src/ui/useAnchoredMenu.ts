import { useLayoutEffect, type RefObject } from "react";

// Portaled menus stay inside the visible viewport, including while the IME is open.
export function useAnchoredMenu({ open, anchorRef, menuRef }: {
  open: boolean;
  anchorRef: RefObject<HTMLElement | null>;
  menuRef: RefObject<HTMLElement | null>;
}) {
  useLayoutEffect(() => {
    if (!open) return;
    const place = () => {
      const trigger = anchorRef.current;
      const menu = menuRef.current;
      if (trigger === null || menu === null) return;
      const viewport = window.visualViewport;
      const left = (viewport?.offsetLeft ?? 0) + 8;
      const top = (viewport?.offsetTop ?? 0) + 8;
      const width = Math.max(0, (viewport?.width ?? window.innerWidth) - 16);
      const height = Math.max(0, (viewport?.height ?? window.innerHeight) - 16);
      menu.style.maxWidth = `${width}px`;
      menu.style.maxHeight = `${height}px`;
      const anchor = trigger.getBoundingClientRect();
      const bounds = menu.getBoundingClientRect();
      const preferredTop = anchor.bottom + 4 + bounds.height <= top + height
        ? anchor.bottom + 4
        : anchor.top - bounds.height - 4;
      menu.style.left = `${Math.max(left, Math.min(anchor.right - bounds.width, left + width - bounds.width))}px`;
      menu.style.top = `${Math.max(top, Math.min(preferredTop, top + height - bounds.height))}px`;
    };
    place();
    window.addEventListener("resize", place);
    window.addEventListener("scroll", place, true);
    window.visualViewport?.addEventListener("resize", place);
    window.visualViewport?.addEventListener("scroll", place);
    return () => {
      window.removeEventListener("resize", place);
      window.removeEventListener("scroll", place, true);
      window.visualViewport?.removeEventListener("resize", place);
      window.visualViewport?.removeEventListener("scroll", place);
    };
  }, [anchorRef, menuRef, open]);
}
