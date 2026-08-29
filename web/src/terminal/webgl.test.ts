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
  it("keeps the DOM renderer when WebGL is unavailable", async () => {
    const terminal = { loadAddon: vi.fn() };
    await expect(attachWebglRenderer(terminal, () => false)).resolves.toBeNull();
    expect(terminal.loadAddon).not.toHaveBeenCalled();
  });

  it("disposes the WebGL addon after context loss so xterm falls back safely", async () => {
    const terminal = { loadAddon: vi.fn() };
    const { addon, loseContext } = addonHarness();
    const attached = await attachWebglRenderer(
      terminal,
      () => true,
      async () => addon as never,
    );

    expect(terminal.loadAddon).toHaveBeenCalledWith(addon);
    loseContext();
    expect(addon.dispose).toHaveBeenCalledOnce();
    attached?.dispose();
  });
});
