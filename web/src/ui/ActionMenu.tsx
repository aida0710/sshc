import { useRef, useState } from "react";
import { createPortal } from "react-dom";
import { Icon } from "./icons";
import { useAnchoredMenu } from "./useAnchoredMenu";
import { useDismissibleLayer } from "./useDismissibleLayer";
import { useMenuKeyboard } from "./useMenuKeyboard";

export type ActionMenuItem = {
  label: string;
  onSelect: () => void;
  tone?: "danger";
  disabled?: boolean;
};

type ActionMenuProps = {
  // The accessible name of the "…" button, naming the thing the actions act on.
  label: string;
  items: readonly ActionMenuItem[];
  // Where the button sits decides its size and background.
  triggerClassName: string;
  iconClassName?: string;
};

// A "…" button that opens the actions for one item as a menu. The menu follows
// the temporary layer contract of useDismissibleLayer and the keys of
// useMenuKeyboard, and is portaled so that a card's rounded corners or a
// scrolling list do not clip it.
export function ActionMenu({ label, items, triggerClassName, iconClassName = "size-4" }: ActionMenuProps) {
  const [open, setOpen] = useState(false);
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
      <button
        ref={triggerRef}
        type="button"
        aria-label={label}
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((current) => !current)}
        className={triggerClassName}
      >
        <Icon name="moreHorizontal" className={iconClassName} />
      </button>
      {open ? createPortal(
        <div ref={menuRef} role="menu" className="fixed z-50 w-56 overflow-y-auto rounded-lg border border-line bg-card p-1 shadow-lg">
          {items.map((item) => (
            <button
              key={item.label}
              type="button"
              role="menuitem"
              disabled={item.disabled}
              onClick={() => select(item)}
              className={`block min-h-10 w-full rounded-md px-3 py-2 text-left text-sm hover:bg-select-fill focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:min-h-0 ${item.tone === "danger" ? "text-danger" : "text-ink"}`}
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
