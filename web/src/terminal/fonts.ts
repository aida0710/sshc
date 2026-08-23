
export type Font = { readonly name: string; readonly label: string; readonly stack: string };
export const defaultStack =
  'ui-monospace, SFMono-Regular, "SF Mono", Menlo, "Roboto Mono", "Droid Sans Mono", monospace';

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
