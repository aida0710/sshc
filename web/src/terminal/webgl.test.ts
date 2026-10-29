import { describe, expect, it, vi } from "vitest";
import { attachWebglRenderer, attachWebglWhileShown } from "./webgl";

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

  describe("while the terminal is hidden", () => {
    it("gives the renderer back while hidden and attaches a new one when shown again", async () => {
      const terminal = { loadAddon: vi.fn() };
      const addons: ReturnType<typeof addonHarness>["addon"][] = [];
      const load = vi.fn(async () => {
        const { addon } = addonHarness();
        addons.push(addon);
        return addon as never;
      });
      const renderer = attachWebglWhileShown(terminal, { supported: () => true, load });

      // The renderer is attached once its import and set-up have finished.
      const attached = () => new Promise((resolve) => setTimeout(resolve, 0));
      renderer.shown(true);
      await attached();
      expect(addons).toHaveLength(1);
      renderer.shown(false);
      expect(addons[0]!.dispose).toHaveBeenCalledOnce();

      renderer.shown(true);
      await attached();
      renderer.shown(true);
      await attached();
      expect(load).toHaveBeenCalledTimes(2);
      renderer.dispose();
      expect(addons[1]!.dispose).toHaveBeenCalledOnce();
    });

    it("lets go of a renderer that finished loading after the terminal was hidden", async () => {
      const terminal = { loadAddon: vi.fn() };
      const { addon } = addonHarness();
      let finish: (value: never) => void = () => undefined;
      const load = vi.fn(() => new Promise<never>((resolve) => { finish = resolve; }));
      const renderer = attachWebglWhileShown(terminal, { supported: () => true, load });

      renderer.shown(true);
      renderer.shown(false);
      finish(addon as never);

      await vi.waitFor(() => expect(addon.dispose).toHaveBeenCalledOnce());
    });
  });

  describe("while the renderer loads", () => {
    it("says it is attaching until the renderer is in place, then that the attach has settled", async () => {
      const terminal = { loadAddon: vi.fn() };
      const { addon } = addonHarness();
      let finish: (value: never) => void = () => undefined;
      const load = vi.fn(() => new Promise<never>((resolve) => { finish = resolve; }));
      const webglSettled = vi.fn();
      const renderer = attachWebglWhileShown(terminal, { supported: () => true, load, webglSettled });

      renderer.shown(true);
      expect(renderer.attaching()).toBe(true);
      finish(addon as never);

      await vi.waitFor(() => expect(webglSettled).toHaveBeenCalledOnce());
      expect(renderer.attaching()).toBe(false);
      expect(terminal.loadAddon).toHaveBeenCalledWith(addon);
    });

    it("says the attach has settled when the renderer could not be loaded", async () => {
      const terminal = { loadAddon: vi.fn() };
      const webglSettled = vi.fn();
      const renderer = attachWebglWhileShown(terminal, {
        supported: () => true,
        load: () => Promise.reject(new Error("no WebGL addon")),
        webglSettled,
      });

      renderer.shown(true);

      await vi.waitFor(() => expect(webglSettled).toHaveBeenCalledOnce());
      expect(renderer.attaching()).toBe(false);
    });

    it("does not try again on every fit when WebGL is turned off, only after being hidden and shown", async () => {
      const terminal = { loadAddon: vi.fn() };
      const webglSettled = vi.fn();
      const renderer = attachWebglWhileShown(terminal, { enabled: false, webglSettled });

      renderer.shown(true);
      await vi.waitFor(() => expect(webglSettled).toHaveBeenCalledOnce());
      renderer.shown(true);
      expect(renderer.attaching()).toBe(false);

      renderer.shown(false);
      renderer.shown(true);
      expect(renderer.attaching()).toBe(true);
      await vi.waitFor(() => expect(webglSettled).toHaveBeenCalledTimes(2));
    });

    it("says nothing once the terminal is disposed", async () => {
      const terminal = { loadAddon: vi.fn() };
      const { addon } = addonHarness();
      const webglSettled = vi.fn();
      const renderer = attachWebglWhileShown(terminal, { supported: () => true, load: async () => addon as never, webglSettled });

      renderer.shown(true);
      renderer.dispose();

      await vi.waitFor(() => expect(addon.dispose).toHaveBeenCalledOnce());
      expect(webglSettled).not.toHaveBeenCalled();
    });
  });
});
