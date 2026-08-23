import { describe, expect, it, vi } from "vitest";
import { attachImeKeys, imeKeyCode, withholdFromTerminal } from "./imeKeys";

describe("withholdFromTerminal", () => {
  it("withholds the key the IME uses to say it is handling input", () => {
    expect(withholdFromTerminal(imeKeyCode, false)).toBe(true);
  });

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
