// The arithmetic behind a custom accent colour: what a colour value must look like, which ink reads on it,
// and how far it stands out from the page. Contrast follows WCAG 2 (relative luminance).

// accentColorPattern is the form the engine stores (#rrggbb, lower case), the one <input type="color">
// reports.
const accentColorPattern = /^#[0-9a-f]{6}$/;

export function isAccentColor(value: unknown): value is string {
  return typeof value === "string" && accentColorPattern.test(value);
}

// parseAccentColor reads what someone typed: with or without the #, in either case. Anything else is null.
export function parseAccentColor(typed: string): string | null {
  const trimmed = typed.trim().toLowerCase();
  const value = trimmed.startsWith("#") ? trimmed : `#${trimmed}`;
  return isAccentColor(value) ? value : null;
}

// The two inks a button in the accent colour may carry: the darkest and the lightest the presets put on
// theirs in index.css. They are needed here as values to measure contrast against.
const darkInk = "#111214"; // palette-exempt
const lightInk = "#ffffff"; // palette-exempt

export function readableInkOn(color: string): string {
  return contrastRatio(color, darkInk) >= contrastRatio(color, lightInk) ? darkInk : lightInk;
}

// A colour shown as an item's frame or a button's fill must stand this far from the page under it (WCAG's
// 3:1 for parts of the interface), or the current item and the focus ring are hard to see.
export const minimumAccentContrast = 3;

export function contrastRatio(first: string, second: string): number {
  const [lighter, darker] = [relativeLuminance(first), relativeLuminance(second)].sort((a, b) => b - a) as [number, number];
  return (lighter + 0.05) / (darker + 0.05);
}

function relativeLuminance(color: string): number {
  const channels = [1, 3, 5].map((start) => Number.parseInt(color.slice(start, start + 2), 16) / 255);
  const [red, green, blue] = channels.map((value) => (value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4)) as [number, number, number];
  return 0.2126 * red + 0.7152 * green + 0.0722 * blue;
}
