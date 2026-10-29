// The font sizes the engine accepts (EmbeddedTerminal.fontSize), in CSS pixels.
export const fontSizeRange = { min: 8, max: 32 } as const;

function clamped(size: number): number {
  return Math.min(fontSizeRange.max, Math.max(fontSizeRange.min, size));
}

// steppedFontSize is size moved by steps whole pixels, kept within the range.
export function steppedFontSize(size: number, steps: number): number {
  return clamped(Math.round(size) + steps);
}

// pinchedFontSize is the size a pinch makes of the size it started at: the fingers' gap grew by scale, and the
// text grows with it, to a whole pixel.
export function pinchedFontSize(startSize: number, scale: number): number {
  return clamped(Math.round(startSize * scale));
}
