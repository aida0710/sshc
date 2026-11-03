import { symbolFontFamily } from "../ui/monospaceFonts";

// One of the symbols, for asking the browser to fetch the font that draws it.
const sampleSymbol = "⏵";
// The size does not matter to the browser's loading; a size is required.
const fontDescription = `16px "${symbolFontFamily}"`;

// redrawOnceSymbolFontLoads redraws a terminal when the font of its symbols arrives. The browser fetches the font
// only when the symbol is first drawn, and until then draws a box. xterm's WebGL renderer keeps what it drew in its
// texture atlas, so the box would stay for as long as the terminal does. It returns a function that stops waiting.
export function redrawOnceSymbolFontLoads(view: { clearTextureAtlas(): void }): () => void {
  const fontSet = document.fonts;
  if (fontSet === undefined || fontSet.check(fontDescription, sampleSymbol)) return () => {};
  let waiting = true;
  void fontSet.load(fontDescription, sampleSymbol).then(() => {
    if (waiting) view.clearTextureAtlas();
  }).catch(() => undefined);
  return () => { waiting = false; };
}
