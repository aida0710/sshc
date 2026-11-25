import { useRef, type ReactNode, type RefObject } from "react";
import { createPortal } from "react-dom";
import { menuSurface } from "./menuStyle";
import { useAnchoredMenu, type MenuAnchor } from "./useAnchoredMenu";
import { useDismissibleLayer } from "./useDismissibleLayer";
import { useMenuKeyboard } from "./useMenuKeyboard";

// MenuWidth はメニューの幅。ふつうは regular。スイッチや選ぶ印が名前の右に並ぶメニュー（ターミナルの「…」）は wide。
export type MenuWidth = "regular" | "wide";

const menuWidths: Record<MenuWidth, string> = {
  regular: "w-56",
  wide: "w-72",
};

// MenuPlacement は、メニューをどこに開くか。
// - anchorRef: 開いたボタンの横。align が end（既定）ならボタンの右端に、start なら左端にそろえる。閉じたら
//   ボタンへフォーカスを戻す。
// - point: 右クリックした点。閉じたら、開く前にフォーカスがあった所へ戻す。
type MenuPlacement =
  | { anchorRef: RefObject<HTMLElement | null>; align?: "start" | "end"; point?: never }
  | { point: { x: number; y: number }; anchorRef?: never; align?: never };

// MenuPanel は、メニューの枠。開いている間だけ描く。画面の上に固定して置き（下に入らなければ上へ）、カードの角や
// 一覧のスクロールで切れないよう body に出す。外を押す・Esc・Android の戻るで onClose を呼び、矢印キーで項目を
// 移る。中身は MenuItems の部品で書く。
export function MenuPanel({ label, width = "regular", onClose, children, ...placement }: MenuPlacement & {
  // label はメニューの名前。開いたボタンと同じく、何の操作かを言う。
  label: string;
  width?: MenuWidth;
  onClose: () => void;
  children: ReactNode;
}) {
  const menuRef = useRef<HTMLDivElement>(null);
  const anchorRef = placement.anchorRef;
  const anchor: MenuAnchor = anchorRef === undefined
    ? { point: placement.point }
    : { element: anchorRef, align: placement.align ?? "end" };
  useDismissibleLayer({
    open: true,
    containerRefs: anchorRef === undefined ? [menuRef] : [menuRef, anchorRef],
    onDismiss: onClose,
    ...(anchorRef === undefined ? {} : { returnFocusRef: anchorRef }),
  });
  useMenuKeyboard({ open: true, menuRef, onClose });
  useAnchoredMenu({ open: true, anchor, menuRef });

  return createPortal(
    <div ref={menuRef} role="menu" aria-label={label} className={`fixed z-50 overflow-y-auto ${menuWidths[width]} ${menuSurface}`}>
      {children}
    </div>,
    document.body,
  );
}
