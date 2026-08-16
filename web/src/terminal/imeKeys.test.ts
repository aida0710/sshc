import { describe, expect, it, vi } from "vitest";
import { attachImeKeys, imeKeyCode, withholdFromTerminal } from "./imeKeys";

describe("withholdFromTerminal", () => {
  // **これが重複の入口である。** 確定した 1 文字ごとに Android は 229 を送り、
  // xterm はそのたびに setTimeout(0) で textarea の前後を比べる——消さないまま。
  it("withholds the key the IME uses to say it is handling input", () => {
    expect(withholdFromTerminal(imeKeyCode, false)).toBe(true);
  });

  // 変換の最中は渡す。日本語の入力は同じ 229 を使うが、あちらは
  // compositionstart から compositionend までを xterm が自分で数えている。
  it("hands it over while a composition is running", () => {
    expect(withholdFromTerminal(imeKeyCode, true)).toBe(false);
  });

  it("never touches an ordinary key", () => {
    expect(withholdFromTerminal(13, false)).toBe(false);
    expect(withholdFromTerminal(65, false)).toBe(false);
    expect(withholdFromTerminal(27, true)).toBe(false);
  });
});

describe("attachImeKeys", () => {
  function harness() {
    const container = document.createElement("div");
    const textarea = document.createElement("textarea");
    container.appendChild(textarea);
    document.body.appendChild(container);
    // xterm の listener の代わり。**textarea に直接付いている**ので、
    // 先回りできるのは祖先の capture だけである。
    const terminal = vi.fn();
    textarea.addEventListener("keydown", terminal);
    const detach = attachImeKeys({ container, textarea });
    const press = (keyCode: number) => {
      const event = new KeyboardEvent("keydown", { bubbles: true });
      Object.defineProperty(event, "keyCode", { value: keyCode });
      textarea.dispatchEvent(event);
    };
    return { textarea, terminal, detach, press };
  }

  it("keeps the IME key away from the terminal", () => {
    const { terminal, detach, press } = harness();
    press(imeKeyCode);
    expect(terminal).not.toHaveBeenCalled();
    detach();
  });

  it("lets every other key through", () => {
    const { terminal, detach, press } = harness();
    press(13);
    press(65);
    expect(terminal).toHaveBeenCalledTimes(2);
    detach();
  });

  // 変換が始まったら手を引く。終われば、また取り上げる。
  it("stands aside for the length of a composition", () => {
    const { textarea, terminal, detach, press } = harness();
    textarea.dispatchEvent(new CompositionEvent("compositionstart", { bubbles: true }));
    press(imeKeyCode);
    expect(terminal).toHaveBeenCalledTimes(1);

    textarea.dispatchEvent(new CompositionEvent("compositionend", { bubbles: true }));
    press(imeKeyCode);
    expect(terminal).toHaveBeenCalledTimes(1);
    detach();
  });

  it("gives the key back once detached", () => {
    const { terminal, detach, press } = harness();
    detach();
    press(imeKeyCode);
    expect(terminal).toHaveBeenCalledTimes(1);
  });
});
