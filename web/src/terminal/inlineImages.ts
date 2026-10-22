import { ImageAddon } from "@xterm/addon-image";
import type { Terminal } from "@xterm/xterm";
import { attachKittyImageGuard, maxKittyControlLength } from "./kittyImageGuard";

// Every open terminal keeps its own image store and several sessions can be
// open at once, on a phone too, so each store is far below the addon's 128 MB.
const imageStorageLimitMiB = 32;
// The addon's default: 4096 × 4096 pixels, 64 MiB once decoded to RGBA.
const imagePixelLimit = 4096 * 4096;
// xterm hands an APC handler registered through its public API at most this
// many characters of one sequence and skips the handler for longer ones.
const xtermApcPayloadLimit = 10_000_000;
// The kitty image guard must see every image the addon accepts, so the base64
// of one image (4 characters per 3 bytes) must fit in what xterm hands over
// after the control data and its ";".
const kittyImageByteLimit = Math.floor((xtermApcPayloadLimit - maxKittyControlLength - 1) / 4) * 3;

// attachInlineImages draws SIXEL (DCS), iTerm2 inline images (OSC 1337 File=)
// and kitty graphics (APC G, which Claude Code uses) in the terminal, and
// answers the kitty graphics query programs send to detect support. The SIXEL
// decoder is WebAssembly, which is why the engine's CSP allows
// 'wasm-unsafe-eval'.
export function attachInlineImages(view: Terminal): { dispose(): void } {
  const addon = new ImageAddon({
    storageLimit: imageStorageLimitMiB,
    pixelLimit: imagePixelLimit,
    kittySizeLimit: kittyImageByteLimit,
  });
  view.loadAddon(addon);
  const kittyGuard = attachKittyImageGuard({
    parser: view.parser,
    images: addon,
    limits: { pixelLimit: imagePixelLimit, imageByteLimit: kittyImageByteLimit },
  });
  return {
    dispose() {
      kittyGuard.dispose();
      addon.dispose();
    },
  };
}
