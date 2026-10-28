import { SerializeAddon } from "@xterm/addon-serialize";
import { Terminal } from "@xterm/xterm";
import { afterEach, describe, expect, it, vi } from "vitest";
import { selectionWithColors } from "./richCopy";

const cleanups: (() => void)[] = [];
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup();
  vi.unstubAllGlobals();
});

describe("copying with colors", () => {
  it("gives the selection as plain text and as HTML that keeps its colors", async () => {
    const view = new Terminal({ cols: 40, rows: 5, allowProposedApi: true });
    const serializer = new SerializeAddon();
    view.loadAddon(serializer);
    // Selecting needs the terminal on the page, which asks the window for its pixel ratio.
    vi.stubGlobal("matchMedia", () => ({ matches: false, addEventListener: () => undefined, removeEventListener: () => undefined, addListener: () => undefined, removeListener: () => undefined }));
    const host = document.body.appendChild(document.createElement("div"));
    view.open(host);
    cleanups.push(() => {
      view.dispose();
      host.remove();
    });
    await new Promise<void>((resolve) => view.write("plain \u001b[31mred\u001b[0m \u001b[1mbold\u001b[0m", resolve));

    expect(selectionWithColors(view, serializer)).toBeNull();
    view.select(0, 0, 15);
    const copied = selectionWithColors(view, serializer);

    expect(copied?.text).toBe("plain red bold");
    expect(copied?.html).toContain("red");
    expect(copied?.html).toMatch(/color: ?#[0-9a-f]{6}/i);
    expect(copied?.html).toContain("font-weight: bold");
  });
});
