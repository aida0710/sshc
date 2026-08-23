
export type Palette = { readonly name: string; readonly label: string };

export const palettes: readonly Palette[] = [
  { name: "solarized-dark", label: "Solarized Dark" },
  { name: "solarized-light", label: "Solarized Light" },
  { name: "dracula", label: "Dracula" },
  { name: "nord", label: "Nord" },
  { name: "gruvbox-dark", label: "Gruvbox Dark" },
  { name: "one-dark", label: "One Dark" },
];
export function knownPalette(name: string): Palette | null {
  return palettes.find((palette) => palette.name === name) ?? null;
}
