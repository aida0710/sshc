import { Terminal } from "@xterm/xterm";
import { afterEach, describe, expect, it, vi } from "vitest";
import { attachLigatures, ligatureRanges } from "./ligatures";

const cleanups: (() => void)[] = [];
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup();
});

describe("ligatureRanges", () => {
  it("合字を含む記号の並びを1つにまとめ、文字や空白で区切る", () => {
    expect(ligatureRanges("a -> b != c")).toEqual([[2, 4], [7, 9]]);
    expect(ligatureRanges("x === y && z")).toEqual([[2, 5], [8, 10]]);
    expect(ligatureRanges("<!-- note -->")).toEqual([[0, 4], [10, 13]]);
    expect(ligatureRanges("a&&b")).toEqual([[1, 3]]);
  });

  it("合字の前後に続く記号も同じまとまりにして、字体の規則に任せる", () => {
    expect(ligatureRanges("a=>{b")).toEqual([[1, 4]]);
    expect(ligatureRanges("x ||= y")).toEqual([[2, 5]]);
  });

  it("合字の無い記号の並びと、区切り線のような長い並びはまとめない", () => {
    expect(ligatureRanges("f() [x]")).toEqual([]);
    expect(ligatureRanges("=".repeat(16))).toEqual([[0, 16]]);
    expect(ligatureRanges("=".repeat(17))).toEqual([]);
    expect(ligatureRanges("")).toEqual([]);
  });
});

describe("attachLigatures", () => {
  function openedTerminal() {
    vi.stubGlobal("matchMedia", () => ({ matches: false, addEventListener: () => undefined, removeEventListener: () => undefined, addListener: () => undefined, removeListener: () => undefined }));
    const view = new Terminal({ allowProposedApi: true });
    const element = document.createElement("div");
    document.body.appendChild(element);
    view.open(element);
    cleanups.push(() => {
      view.dispose();
      element.remove();
    });
    return view;
  }

  it("入れると文字のまとまりを xterm.js に渡し、切ると外す", () => {
    const view = openedTerminal();
    const register = vi.spyOn(view, "registerCharacterJoiner");
    const deregister = vi.spyOn(view, "deregisterCharacterJoiner");
    const drawing = attachLigatures(view, false);
    expect(register).not.toHaveBeenCalled();
    expect(view.element?.hasAttribute("data-ligatures")).toBe(false);

    drawing.set(true);
    drawing.set(true);
    expect(register).toHaveBeenCalledOnce();
    expect(view.element?.hasAttribute("data-ligatures")).toBe(true);

    drawing.dispose();
    expect(deregister).toHaveBeenCalledOnce();
    expect(view.element?.hasAttribute("data-ligatures")).toBe(false);
  });
});
