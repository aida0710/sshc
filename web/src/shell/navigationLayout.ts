import type { PresetColumnWidth } from "../settings/usePresetColumnWidth";

// The desktop navigation column: wide enough for section names with badges,
// never so wide that a laptop loses its content area.
export const navigationWidth: PresetColumnWidth = { field: "navigationWidth", fallback: 252, minimum: 192, maximum: 384 };
