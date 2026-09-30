import type { StoredColumnWidth } from "../ui/useStoredColumnWidth";
import { localStorageKeys } from "../ui/browserStorageKeys";

// The desktop navigation column: wide enough for section names with badges,
// never so wide that a laptop loses its content area.
export const navigationWidth: StoredColumnWidth = { key: localStorageKeys.navigationWidth, fallback: 240, minimum: 192, maximum: 384 };
