import type { WebglAddon } from "@xterm/addon-webgl";

type Disposable = { dispose(): void };

type WebglTerminal = {
  loadAddon(addon: WebglAddon): void;
};

type WebglSupport = () => boolean;

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
  supported: WebglSupport = browserSupportsWebgl,
  load: () => Promise<WebglAddon> = async () => {
    const { WebglAddon } = await import("@xterm/addon-webgl");
    return new WebglAddon();
  },
): Promise<Disposable | null> {
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
