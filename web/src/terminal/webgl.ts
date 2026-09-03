import type { WebglAddon } from "@xterm/addon-webgl";

type Disposable = { dispose(): void };

type WebglTerminal = {
  loadAddon(addon: WebglAddon): void;
};

type WebglSupport = () => boolean;

type WebglOptions = {
  backgroundImage?: boolean;
  supported?: WebglSupport;
  load?: () => Promise<WebglAddon>;
};

export function browserSupportsWebgl(): boolean {
  // jsdom exposes the canvas API but reports getContext through its virtual
  // console instead of implementing it. Skip before touching the method so
  // unit tests exercise the deterministic DOM-renderer path without noise.
  if (typeof navigator !== "undefined" && /\bjsdom\b/iu.test(navigator.userAgent)) return false;
  if (typeof WebGLRenderingContext === "undefined" && typeof WebGL2RenderingContext === "undefined") return false;
  try {
    const canvas = document.createElement("canvas");
    return canvas.getContext("webgl2") !== null || canvas.getContext("webgl") !== null;
  } catch {
    return false;
  }
}

export async function attachWebglRenderer(
  terminal: WebglTerminal,
  {
    backgroundImage = false,
    supported = browserSupportsWebgl,
    load = async () => {
      const { WebglAddon } = await import("@xterm/addon-webgl");
      return new WebglAddon();
    },
  }: WebglOptions = {},
): Promise<Disposable | null> {
  // The WebGL addon repaints by drawing the terminal background over the
  // previous frame. A fully transparent background cannot erase old glyphs
  // reliably (notably in Chromium on macOS), so edited command lines leave
  // characters behind. Keep xterm's DOM renderer for image-backed terminals;
  // it composites and clears transparent cells correctly.
  if (backgroundImage) return null;
  if (!supported()) return null;
  let addon: WebglAddon;
  try {
    addon = await load();
    terminal.loadAddon(addon);
  } catch {
    return null;
  }
  const contextLoss = addon.onContextLoss(() => {
    contextLoss.dispose();
    addon.dispose();
  });
  return {
    dispose() {
      contextLoss.dispose();
      addon.dispose();
    },
  };
}
