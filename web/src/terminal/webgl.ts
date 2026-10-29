import type { WebglAddon } from "@xterm/addon-webgl";
import type { IDisposable } from "@xterm/xterm";

type WebglTerminal = {
  loadAddon(addon: WebglAddon): void;
};

type WebglSupport = () => boolean;

type WebglOptions = {
  enabled?: boolean;
  backgroundImage?: boolean;
  supported?: WebglSupport;
  load?: () => Promise<WebglAddon>;
};

// Asked once per page: every terminal shown would otherwise create a probe
// context, and browsers count live WebGL contexts against a small limit.
let webglSupported: boolean | undefined;

export function browserSupportsWebgl(): boolean {
  webglSupported ??= probeWebgl();
  return webglSupported;
}

function probeWebgl(): boolean {
  // jsdom exposes the canvas API but reports getContext through its virtual
  // console instead of implementing it. Skip before touching the method so
  // unit tests exercise the deterministic DOM-renderer path without noise.
  if (typeof navigator !== "undefined" && /\bjsdom\b/iu.test(navigator.userAgent)) return false;
  if (typeof WebGLRenderingContext === "undefined" && typeof WebGL2RenderingContext === "undefined") return false;
  try {
    const canvas = document.createElement("canvas");
    const context = canvas.getContext("webgl2") ?? canvas.getContext("webgl");
    // Give the probe's context back at once instead of when it is collected.
    context?.getExtension("WEBGL_lose_context")?.loseContext();
    return context !== null;
  } catch {
    return false;
  }
}

export async function attachWebglRenderer(
  terminal: WebglTerminal,
  {
    enabled = true,
    backgroundImage = false,
    supported = browserSupportsWebgl,
    load = async () => {
      const { WebglAddon } = await import("@xterm/addon-webgl");
      return new WebglAddon();
    },
  }: WebglOptions = {},
): Promise<IDisposable | null> {
  // The WebGL addon repaints by drawing the terminal background over the
  // previous frame. A fully transparent background cannot erase old glyphs
  // reliably (notably in Chromium on macOS), so edited command lines leave
  // characters behind. Keep xterm's DOM renderer for image-backed terminals;
  // it composites and clears transparent cells correctly.
  if (!enabled || backgroundImage) return null;
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

type WhileShownOptions = WebglOptions & {
  // webglSettled is called when an attach has ended, with the WebGL renderer in place or not.
  webglSettled?: () => void;
};

// attachWebglWhileShown keeps the WebGL renderer only while the terminal is on
// the screen. A hidden terminal (a session kept after it was left, a pane
// behind Focus mode) gives back its GPU context and glyph textures and draws
// with the DOM renderer, which costs nothing while nothing is displayed.
export function attachWebglWhileShown(terminal: WebglTerminal, options: WhileShownOptions = {}): {
  shown(visible: boolean): void;
  // attaching tells whether the WebGL renderer is loading: until it is in place,
  // the cells are still measured by the renderer it replaces.
  attaching(): boolean;
  dispose(): void;
} {
  let renderer: IDisposable | null = null;
  let attaching = false;
  let wanted = false;
  let disposed = false;
  // unavailable is set when an attach ended without a renderer (WebGL turned off, unsupported
  // or not loaded). Each fit asks for the renderer, so it is tried again only once the
  // terminal has been hidden and shown.
  let unavailable = false;
  const settle = () => {
    if (disposed) return;
    if (!wanted) {
      renderer?.dispose();
      renderer = null;
      unavailable = false;
      return;
    }
    if (renderer !== null || attaching || unavailable) return;
    attaching = true;
    void attachWebglRenderer(terminal, options).then((attached) => {
      attaching = false;
      if (disposed) {
        attached?.dispose();
        return;
      }
      if (wanted) {
        renderer = attached;
        unavailable = attached === null;
      } else {
        attached?.dispose();
      }
      options.webglSettled?.();
    });
  };
  return {
    shown(visible) {
      wanted = visible;
      settle();
    },
    attaching: () => attaching,
    dispose() {
      disposed = true;
      renderer?.dispose();
      renderer = null;
    },
  };
}
