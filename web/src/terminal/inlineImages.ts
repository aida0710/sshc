import { ImageAddon } from "@xterm/addon-image";
import type { Terminal } from "@xterm/xterm";

// Every open terminal keeps its own image store and several sessions can be
// open at once, on a phone too, so each store is far below the addon's 128 MB.
const imageStorageLimitMiB = 32;

// attachInlineImages draws SIXEL (DCS), iTerm2 inline images (OSC 1337 File=)
// and kitty graphics (APC G, which Claude Code uses) in the terminal, and
// answers the kitty graphics query programs send to detect support. The SIXEL
// decoder is WebAssembly, which is why the engine's CSP allows
// 'wasm-unsafe-eval'.
export function attachInlineImages(view: Terminal): { dispose(): void } {
  const addon = new ImageAddon({ storageLimit: imageStorageLimitMiB });
  view.loadAddon(addon);
  return addon;
}
