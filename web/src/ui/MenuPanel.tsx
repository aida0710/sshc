import { useRef, type ReactNode, type RefObject } from "react";
import { createPortal } from "react-dom";
import { menuSurface } from "./menuStyle";
import { useAnchoredMenu } from "./useAnchoredMenu";
import { useDismissibleLayer } from "./useDismissibleLayer";
import { useMenuKeyboard } from "./useMenuKeyboard";

// MenuWidth はメニューの幅。ふつうは regular。スイッチや選ぶ印が名前の右に並ぶメニュー（ターミナルの「…」）は wide。
export type MenuWidth = "regular" | "wide";

const menuWidths: Record<MenuWidth, string> = {
  regular: "w-56",
  wide: "w-72",
};

// MenuPanel は、ボタンから開くメニューの枠。開いている間だけ描く。開いたボタン（anchorRef）の横に、画面の上に
// 固定して置き（下に入らなければ上へ）、カードの角や一覧のスクロールで切れないよう body に出す。外を押す・Esc・
// Android の戻るで onClose を呼び、閉じたらボタンへフォーカスを戻す。矢印キーで項目を移る。中身は MenuItems の
// 部品で書く。
export function MenuPanel({ label, anchorRef, width = "regular", onClose, children }: {
  // label はメニューの名前。開いたボタンと同じく、何の操作かを言う。
  label: string;
  anchorRef: RefObject<HTMLElement | null>;
  width?: MenuWidth;
  onClose: () => void;
  children: ReactNode;
}) {
  const menuRef = useRef<HTMLDivElement>(null);
  useDismissibleLayer({
    open: true,
    containerRefs: [menuRef, anchorRef],
    onDismiss: onClose,
    returnFocusRef: anchorRef,
  });
  useMenuKeyboard({ open: true, menuRef, onClose });
  useAnchoredMenu({ open: true, anchorRef, menuRef });

  return createPortal(
    <div ref={menuRef} role="menu" aria-label={label} className={`fixed z-50 overflow-y-auto ${menuWidths[width]} ${menuSurface}`}>
      {children}
    </div>,
    document.body,
  );
}
