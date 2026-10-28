import { describe, expect, it, vi } from "vitest";
import { attachWebglRenderer } from "./webgl";

function addonHarness() {
  let contextLost: () => void = () => undefined;
  const addon = {
    onContextLoss: vi.fn((callback: () => void) => {
      contextLost = callback;
      return { dispose: vi.fn() };
    }),
    dispose: vi.fn(),
  };
  return { addon, loseContext: () => contextLost() };
}

describe("WebGL terminal renderer", () => {
  it("keeps the DOM renderer when the user disables WebGL", async () => {
    const terminal = { loadAddon: vi.fn() };
    const load = vi.fn();

    await expect(attachWebglRenderer(terminal, {
      enabled: false,
      supported: () => true,
      load,
    })).resolves.toBeNull();
    expect(load).not.toHaveBeenCalled();
    expect(terminal.loadAddon).not.toHaveBeenCalled();
  });

  it("keeps the DOM renderer when WebGL is unavailable", async () => {
    const terminal = { loadAddon: vi.fn() };
    await expect(attachWebglRenderer(terminal, { supported: () => false })).resolves.toBeNull();
    expect(terminal.loadAddon).not.toHaveBeenCalled();
  });

  it("keeps the DOM renderer when a transparent image background is configured", async () => {
    const terminal = { loadAddon: vi.fn() };
    const supported = vi.fn(() => true);
    const load = vi.fn(async () => addonHarness().addon as never);

    await expect(attachWebglRenderer(terminal, { backgroundImage: true, supported, load })).resolves.toBeNull();

    expect(supported).not.toHaveBeenCalled();
    expect(load).not.toHaveBeenCalled();
    expect(terminal.loadAddon).not.toHaveBeenCalled();
  });

  it("disposes the WebGL addon after context loss so xterm falls back safely", async () => {
    const terminal = { loadAddon: vi.fn() };
    const { addon, loseContext } = addonHarness();
    const attached = await attachWebglRenderer(
      terminal,
      { supported: () => true, load: async () => addon as never },
    );

    expect(terminal.loadAddon).toHaveBeenCalledWith(addon);
    loseContext();
    expect(addon.dispose).toHaveBeenCalledOnce();
    attached?.dispose();
  });

  it("asks the browser for WebGL once a page and gives the probe's context back", async () => {
    vi.resetModules();
    vi.stubGlobal("navigator", { userAgent: "Mozilla/5.0 Chrome/156" });
    vi.stubGlobal("WebGL2RenderingContext", class {});
    const loseContext = vi.fn();
    const getContext = vi.fn(() => ({ getExtension: (name: string) => (name === "WEBGL_lose_context" ? { loseContext } : null) }));
    const createElement = vi.spyOn(document, "createElement").mockReturnValue({ getContext } as never);
    try {
      const { browserSupportsWebgl } = await import("./webgl");

      expect(browserSupportsWebgl()).toBe(true);
      expect(browserSupportsWebgl()).toBe(true);
      expect(getContext).toHaveBeenCalledOnce();
      expect(loseContext).toHaveBeenCalledOnce();
    } finally {
      createElement.mockRestore();
      vi.unstubAllGlobals();
    }
  });
});
