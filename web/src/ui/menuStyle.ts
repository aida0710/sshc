// The look of every menu: the floating panel and its items. ActionMenu uses it, and so do the menus
// that place themselves (SFTP's folder, create and selection menus, the transfer manager's actions, the
// terminal's menu and its rows' menus), so a menu looks the same whichever one opens.

// menuSurface is the panel. The caller adds where it sits and how wide it is.
export const menuSurface = "rounded-lg border border-line bg-card p-1 shadow-float";

// Every item's size, hover and focus. Items are taller on touch layouts, where they are tapped. The
// horizontal padding is left to each kind of item. A disabled item takes index.css's look for :disabled.
const itemBase = "min-h-10 w-full rounded-md py-2 text-left text-sm hover:bg-hover focus:bg-select-fill focus:outline-none desktop-layout:min-h-0";

// menuItemClass is one item.
export function menuItemClass(tone?: "danger"): string {
  return `block px-3 ${itemBase} ${tone === "danger" ? "text-danger" : "text-ink"}`;
}

// menuChoiceItemClass is an item that is on or off (menuitemcheckbox, menuitemradio): its name, then its
// mark on the right. indent sets a choice under the one it belongs to.
export function menuChoiceItemClass({ indent = false }: { indent?: boolean } = {}): string {
  return `flex items-center justify-between gap-3 pr-3 ${indent ? "pl-7" : "pl-3"} ${itemBase} text-ink`;
}

// menuGroupClass sets a group of items apart from those above it, under menuGroupHeadingClass.
export const menuGroupClass = "mt-1 border-t border-line pt-1";
export const menuGroupHeadingClass = "px-3 py-1 text-[11px] text-ink-faint";
