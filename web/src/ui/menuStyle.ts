// The look of every menu: the floating panel and its items. ActionMenu uses it, and so do the menus
// that place themselves (SFTP's folder, create and selection menus, the transfer manager's actions),
// so a menu looks the same whichever one opens.

// menuSurface is the panel. The caller adds where it sits and how wide it is.
export const menuSurface = "rounded-lg border border-line bg-card p-1 shadow-lg";

// menuItemClass is one item. Items are taller on touch layouts, where they are tapped.
export function menuItemClass(tone?: "danger"): string {
  return `block min-h-10 w-full rounded-md px-3 py-2 text-left text-sm hover:bg-hover focus:bg-select-fill focus:outline-none desktop-layout:min-h-0 ${
    tone === "danger" ? "text-danger" : "text-ink"
  }`;
}
