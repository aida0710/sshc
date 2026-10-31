import { localStorageKeys } from "../ui/browserStorageKeys";
import type { ColumnWidth } from "../ui/useColumnWidth";

// The desktop navigation column: wide enough for section names with badges,
// never so wide that a laptop loses its content area.
export const navigationWidth: ColumnWidth = { key: localStorageKeys.navigationWidth, fallback: 252, minimum: 192, maximum: 384 };
