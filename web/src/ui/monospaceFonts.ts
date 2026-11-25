// The monospace fonts the terminal and the editors draw with.
export type Font = { readonly name: string; readonly label: string; readonly stack: string };
// Chrome on Windows ignores ui-monospace, and a Japanese Windows resolves the generic monospace
// to BIZ UDGothic or MS Gothic, which draw \ as ¥ and narrow the cells. Cascadia Mono ships with
// Windows 11 and Consolas with every Windows; they come last so macOS, Android and Linux keep their font.
// The bundled symbols come after every text font and cover only what Android's fonts lack or draw as colour emoji
// (index.css).
export const symbolFontFamily = "sshc Terminal Symbols";
export const defaultStack =
  `ui-monospace, SFMono-Regular, "SF Mono", Menlo, "Roboto Mono", "Droid Sans Mono", "Cascadia Mono", Consolas, "${symbolFontFamily}", monospace`;

export const fonts: readonly Font[] = [
  {
    name: "jetbrains-mono",
    label: "JetBrains Mono",
    stack: `"JetBrains Mono", ${defaultStack}`,
  },
  // Japanese drawn with BIZ UDGothic, a universal-design font. Its own Latin letters are not used: it draws ~ at
  // the top of the line, and its thin hyphen vanishes at 13px on a screen of ordinary density. The browser fetches
  // every family of the stack when the terminal asks for the font (FontFaceSet.load), so both are there before the
  // first row is drawn.
  {
    name: "jetbrains-mono-biz-udgothic",
    label: "JetBrains Mono + BIZ UDGothic",
    stack: `"JetBrains Mono", "BIZ UDGothic", ${defaultStack}`,
  },
];
export function knownFont(name: string): Font | null {
  return fonts.find((font) => font.name === name) ?? null;
}
export function fontStack(name: string): string {
  return knownFont(name)?.stack ?? defaultStack;
}
