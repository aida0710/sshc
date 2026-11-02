import { afterEach, describe, expect, it } from "vitest";
import { isInTerminalNotice, isTypingElsewhere } from "./terminalFocus";

afterEach(() => {
  document.body.innerHTML = "";
});

describe("isTypingElsewhere", () => {
  it("is true while a text field outside the terminal holds the focus", () => {
    document.body.innerHTML = `<section id="terminal"><textarea></textarea></section><input aria-label="New name">`;
    document.querySelector<HTMLInputElement>("input")!.focus();

    expect(isTypingElsewhere(document.querySelector("#terminal"))).toBe(true);
  });

  it("is false when the terminal's own input or a button holds the focus", () => {
    document.body.innerHTML = `<section id="terminal"><textarea></textarea></section><button>New session</button>`;
    const terminal = document.querySelector<HTMLElement>("#terminal");

    document.querySelector<HTMLTextAreaElement>("textarea")!.focus();
    expect(isTypingElsewhere(terminal)).toBe(false);

    document.querySelector<HTMLButtonElement>("button")!.focus();
    expect(isTypingElsewhere(terminal)).toBe(false);
  });
});

describe("isInTerminalNotice", () => {
  it("is true while a card over the terminal or one of its buttons holds the focus", () => {
    document.body.innerHTML = `<section id="terminal"><textarea></textarea><section data-terminal-notice="question" tabindex="-1"><button>Cancel</button></section></section>`;
    document.querySelector<HTMLElement>("[data-terminal-notice]")!.focus();
    expect(isInTerminalNotice()).toBe(true);
    document.querySelector<HTMLButtonElement>("button")!.focus();
    expect(isInTerminalNotice()).toBe(true);
    document.querySelector<HTMLTextAreaElement>("textarea")!.focus();
    expect(isInTerminalNotice()).toBe(false);
  });
});
