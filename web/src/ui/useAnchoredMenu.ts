import { useLayoutEffect, type RefObject } from "react";

// MenuAnchor は、メニューを何の横に置くか。
// - element: 開いたボタン。align が end ならボタンの右端に、start なら左端にメニューの端をそろえる。
// - point: 右クリックや長押しをした点。メニューの左上をそこに置く。
export type MenuAnchor =
  | { element: RefObject<HTMLElement | null>; align: "start" | "end" }
  | { point: { x: number; y: number } };

// ボタンとメニューの間をあける幅（px）。点に開くときは、押した所にメニューの角を重ねる。
const gapFromButton = 4;
// 画面の端からあける幅（px）。
const viewportMargin = 8;

// Portaled menus stay inside the visible viewport, including while the IME is open. A menu goes below what it
// is placed by, or above it when it does not fit below.
export function useAnchoredMenu({ open, anchor, menuRef }: {
  open: boolean;
  anchor: MenuAnchor;
  menuRef: RefObject<HTMLElement | null>;
}) {
  const element = "element" in anchor ? anchor.element : null;
  const align = "element" in anchor ? anchor.align : "start";
  const pointX = "point" in anchor ? anchor.point.x : 0;
  const pointY = "point" in anchor ? anchor.point.y : 0;

  useLayoutEffect(() => {
    if (!open) return;
    const place = () => {
      const menu = menuRef.current;
      if (menu === null) return;
      const target = element === null ? null : element.current;
      if (element !== null && target === null) return;
      const viewport = window.visualViewport;
      const left = (viewport?.offsetLeft ?? 0) + viewportMargin;
      const top = (viewport?.offsetTop ?? 0) + viewportMargin;
      const width = Math.max(0, (viewport?.width ?? window.innerWidth) - viewportMargin * 2);
      const height = Math.max(0, (viewport?.height ?? window.innerHeight) - viewportMargin * 2);
      menu.style.maxWidth = `${width}px`;
      menu.style.maxHeight = `${height}px`;
      const by = target === null
        ? { left: pointX, right: pointX, top: pointY, bottom: pointY }
        : target.getBoundingClientRect();
      const gap = target === null ? 0 : gapFromButton;
      const bounds = menu.getBoundingClientRect();
      const preferredTop = by.bottom + gap + bounds.height <= top + height
        ? by.bottom + gap
        : by.top - bounds.height - gap;
      const preferredLeft = align === "end" ? by.right - bounds.width : by.left;
      menu.style.left = `${Math.max(left, Math.min(preferredLeft, left + width - bounds.width))}px`;
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
  }, [align, element, menuRef, open, pointX, pointY]);
}
