import { afterEach, describe, expect, it } from "vitest";
import { isInNoticeCard, isTypingElsewhere, isTypingOutsideTerminals } from "./terminalFocus";

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

describe("isTypingOutsideTerminals", () => {
  it("is true while a text field that is not a terminal's holds the focus", () => {
    document.body.innerHTML = `<div data-terminal-host><textarea></textarea></div><input aria-label="New name">`;
    document.querySelector<HTMLInputElement>("input")!.focus();

    expect(isTypingOutsideTerminals()).toBe(true);
  });

  it("is false while any terminal's input or a button holds the focus", () => {
    document.body.innerHTML = `<div data-terminal-host><textarea></textarea></div><button>New session</button>`;

    document.querySelector<HTMLTextAreaElement>("textarea")!.focus();
    expect(isTypingOutsideTerminals()).toBe(false);

    document.querySelector<HTMLButtonElement>("button")!.focus();
    expect(isTypingOutsideTerminals()).toBe(false);
  });
});

describe("isInNoticeCard", () => {
  it("is true while a card over the terminal or one of its buttons holds the focus", () => {
    document.body.innerHTML = `<section id="terminal"><textarea></textarea><section data-notice-card="question" tabindex="-1"><button>Cancel</button></section></section>`;
    document.querySelector<HTMLElement>("[data-notice-card]")!.focus();
    expect(isInNoticeCard()).toBe(true);
    document.querySelector<HTMLButtonElement>("button")!.focus();
    expect(isInNoticeCard()).toBe(true);
    document.querySelector<HTMLTextAreaElement>("textarea")!.focus();
    expect(isInNoticeCard()).toBe(false);
  });
});
