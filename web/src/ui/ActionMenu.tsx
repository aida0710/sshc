import { useRef, useState } from "react";
import { createPortal } from "react-dom";
import { Icon } from "./icons";
import { menuItemClass, menuSurface } from "./menuStyle";
import { useAnchoredMenu } from "./useAnchoredMenu";
import { useDismissibleLayer } from "./useDismissibleLayer";
import { useMenuKeyboard } from "./useMenuKeyboard";
import { IconButton, type IconButtonGround, type IconButtonSize, type IconButtonTone } from "./IconButton";

// revealOnHover は、行やカードの「…」を、載せられる PC の画面（hover-reveal）では乗せたとき（とキーボードで
// 触れたとき）だけ出し、それ以外（スマホの組み方、タッチだけの画面）では常に出すための class である。行やカードの
// 側に group/row を付けて使う。「…」の下に重なるもの（件数、グループ名）は、hover-reveal のときだけ隠れ、
// それ以外では「…」の分の場所を空けて並ぶ。
export const revealOnHover = "transition-opacity duration-150 hover-reveal:opacity-0 hover-reveal:group-hover/row:opacity-100 hover-reveal:group-focus-within/row:opacity-100";

// revealOnHoverAtPcSize は、場所の狭い行（Connections のグループの一覧、サイドバーのセッションの行）のための
// revealOnHover である。PC の組み方の画面では、タッチ対応でも、乗せたとき（とキーボードで触れたとき）だけ出し、
// その間だけ件数や名前の上に重ねる。いつも出して件数や名前の場所を削らない（スマホの組み方では常に出す）。
// 乗せられない画面では、行を長押ししたときのメニューで同じ操作ができる。
// 乗せたかどうかは、ブラウザの答える「乗せられる入力があるか」（any-hover。hover: の variant が見る）ではなく、
// 実際に乗っているか（:hover）で決める。タッチ画面付きの PC は、マウスで操作していても any-hover を none と
// 答えることがあり、隠したまま出せなくなるためである（2026-10-29、押した瞬間しか出ないと報告があった）。
export const revealOnHoverAtPcSize = "transition-opacity duration-150 desktop-layout:opacity-0 desktop-layout:group-[:hover]/row:opacity-100 desktop-layout:group-focus-within/row:opacity-100";

// hideOnHoverAtPcSize は、revealOnHoverAtPcSize で出す「…」に場所を譲るもの（グループの件数）に付ける。
export const hideOnHoverAtPcSize = "desktop-layout:group-[:hover]/row:invisible desktop-layout:group-focus-within/row:invisible";

export type ActionMenuItem = {
  label: string;
  onSelect: () => void;
  tone?: "danger";
  disabled?: boolean;
  // title says why a disabled item cannot be chosen yet.
  title?: string | undefined;
};

type ActionMenuProps = {
  // The accessible name of the "…" button, naming the thing the actions act on.
  label: string;
  items: readonly ActionMenuItem[];
  // 「…」のボタンの大きさと色（IconButton）。置く場所の行や板に合わせる。
  triggerSize: IconButtonSize;
  triggerTone?: IconButtonTone;
  triggerGround?: IconButtonGround;
  // triggerClassName は、乗せたときだけ出す、選んだ行の色など、置く場所の見せ方を足す。
  triggerClassName?: string;
  // triggerTouchDense は、ボタンの押す場所を、詰めた一覧の高さ 36px・幅 44px にする（index.css の data-touch-dense）。
  // 付けなければ、ふつうの 44px 四方になる。
  triggerTouchDense?: boolean;
  iconClassName?: string;
  // 右クリックでも開くときは、開閉を呼び手が持つ。渡さなければこの部品が持つ。
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
};

// A "…" button that opens the actions for one item as a menu. The menu follows
// the temporary layer contract of useDismissibleLayer and the keys of
// useMenuKeyboard, and is portaled so that a card's rounded corners or a
// scrolling list do not clip it.
export function ActionMenu({ label, items, triggerSize, triggerTone = "muted", triggerGround = "hover", triggerClassName = "", triggerTouchDense = false, iconClassName = "size-4", open: controlledOpen, onOpenChange }: ActionMenuProps) {
  const [ownOpen, setOwnOpen] = useState(false);
  const open = controlledOpen ?? ownOpen;
  const setOpen = (next: boolean | ((current: boolean) => boolean)) => {
    const value = typeof next === "function" ? next(open) : next;
    if (onOpenChange !== undefined) onOpenChange(value);
    else setOwnOpen(value);
  };
  const rootRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  useDismissibleLayer({
    open,
    containerRefs: [rootRef, menuRef],
    onDismiss: () => setOpen(false),
    returnFocusRef: triggerRef,
  });
  useMenuKeyboard({ open, menuRef, onClose: () => setOpen(false) });
  useAnchoredMenu({ open, anchorRef: triggerRef, menuRef });

  // Focus goes back to the button before the action runs. A dialog the action
  // opens then returns focus to the button when it closes, and inside a modal
  // it counts as opened from that modal, so the modal stays open behind it.
  function select(item: ActionMenuItem) {
    triggerRef.current?.focus();
    setOpen(false);
    item.onSelect();
  }

  return (
    <div ref={rootRef} className="relative shrink-0">
      <IconButton
        ref={triggerRef}
        label={label}
        size={triggerSize}
        tone={triggerTone}
        ground={triggerGround}
        dense={triggerTouchDense}
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((current) => !current)}
        className={triggerClassName}
      >
        <Icon name="moreHorizontal" className={iconClassName} />
      </IconButton>
      {open ? createPortal(
        <div ref={menuRef} role="menu" className={`fixed z-50 w-56 overflow-y-auto ${menuSurface}`}>
          {items.map((item) => (
            <button
              key={item.label}
              type="button"
              role="menuitem"
              disabled={item.disabled}
              title={item.title}
              onClick={() => select(item)}
              className={menuItemClass(item.tone)}
            >
              {item.label}
            </button>
          ))}
        </div>,
        document.body,
      ) : null}
    </div>
  );
}
