import { afterEach, describe, expect, it, vi } from "vitest";
import { redrawOnceSymbolFontLoads } from "./terminalSymbolFont";

function fontSet({ loaded, load }: { loaded: boolean; load?: () => Promise<unknown> }) {
  Object.defineProperty(document, "fonts", {
    configurable: true,
    value: { check: () => loaded, load: load ?? (async () => []) },
  });
}

afterEach(() => {
  Reflect.deleteProperty(document, "fonts");
});

describe("redrawOnceSymbolFontLoads", () => {
  it("clears the texture atlas when the font arrives, so no box drawn before it stays", async () => {
    let arrive = () => {};
    fontSet({ loaded: false, load: () => new Promise<void>((resolve) => { arrive = resolve; }) });
    const view = { clearTextureAtlas: vi.fn() };

    redrawOnceSymbolFontLoads(view);
    expect(view.clearTextureAtlas).not.toHaveBeenCalled();
    arrive();

    await vi.waitFor(() => expect(view.clearTextureAtlas).toHaveBeenCalledOnce());
  });

  it("has nothing to redraw when the font is already there", async () => {
    fontSet({ loaded: true });
    const view = { clearTextureAtlas: vi.fn() };

    redrawOnceSymbolFontLoads(view);
    await Promise.resolve();

    expect(view.clearTextureAtlas).not.toHaveBeenCalled();
  });

  it("leaves a terminal that closed while the font was on its way", async () => {
    let arrive = () => {};
    fontSet({ loaded: false, load: () => new Promise<void>((resolve) => { arrive = resolve; }) });
    const view = { clearTextureAtlas: vi.fn() };

    const stopWaiting = redrawOnceSymbolFontLoads(view);
    stopWaiting();
    arrive();
    await Promise.resolve();
    await Promise.resolve();

    expect(view.clearTextureAtlas).not.toHaveBeenCalled();
  });

  it("does not fail where the browser has no font loading API or the font cannot be fetched", async () => {
    const view = { clearTextureAtlas: vi.fn() };
    expect(() => redrawOnceSymbolFontLoads(view)).not.toThrow();

    fontSet({ loaded: false, load: () => Promise.reject(new Error("offline")) });
    redrawOnceSymbolFontLoads(view);
    await Promise.resolve();

    expect(view.clearTextureAtlas).not.toHaveBeenCalled();
  });
});
