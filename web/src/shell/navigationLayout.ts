import { localStorageKeys } from "../ui/browserStorageKeys";
import type { ColumnWidth } from "../ui/useColumnWidth";
import type { StoredValue } from "../ui/useStoredValue";

// The desktop navigation column: wide enough for section names with badges,
// never so wide that a laptop loses its content area.
export const navigationWidth: ColumnWidth = { key: localStorageKeys.navigationWidth, fallback: 252, minimum: 192, maximum: 384 };

// navigationShown is whether a PC shows the navigation; its shortcut hides it, leaving a narrow column with the
// button that shows it again (hiddenNavigationWidth). A phone opens and closes its drawer instead.
export type NavigationShown = "shown" | "hidden";
export const navigationShown: StoredValue<NavigationShown> = {
  key: localStorageKeys.navigationHidden,
  fallback: "shown",
  parse: (stored) => (stored === "hidden" ? "hidden" : "shown"),
};
export const hiddenNavigationWidth = "2.5rem";
