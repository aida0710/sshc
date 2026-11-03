
export type Font = { readonly name: string; readonly label: string; readonly stack: string };
// Chrome on Windows ignores ui-monospace, and a Japanese Windows resolves the generic monospace
// to BIZ UDGothic or MS Gothic, which draw \ as ¥ and narrow the cells. Cascadia Mono ships with
// Windows 11 and Consolas with every Windows; they come last so macOS, Android and Linux keep their font.
export const defaultStack =
  'ui-monospace, SFMono-Regular, "SF Mono", Menlo, "Roboto Mono", "Droid Sans Mono", "Cascadia Mono", Consolas, monospace';

export const fonts: readonly Font[] = [
  {
    name: "jetbrains-mono",
    label: "JetBrains Mono",
    stack: `"JetBrains Mono", ${defaultStack}`,
  },
];
export function knownFont(name: string): Font | null {
  return fonts.find((font) => font.name === name) ?? null;
}
export function fontStack(name: string): string {
  return knownFont(name)?.stack ?? defaultStack;
}
