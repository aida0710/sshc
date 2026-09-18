import type { StoredColumnWidth } from "../ui/useStoredColumnWidth";

// The desktop navigation column: wide enough for section names with badges,
// never so wide that a laptop loses its content area.
export const navigationWidth: StoredColumnWidth = { key: "sshc.navigation.width", fallback: 240, minimum: 192, maximum: 384 };
