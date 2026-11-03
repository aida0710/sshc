import { fontStack } from "../ui/monospaceFonts";

// The bundled font, then the same fallback as the terminal: Windows has to reach Cascadia Mono or Consolas
// instead of the browser's generic monospace while the bundled font loads.
export const editorFontFamily = fontStack("jetbrains-mono");
