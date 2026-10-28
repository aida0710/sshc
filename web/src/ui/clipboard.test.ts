import { afterEach, describe, expect, it, vi } from "vitest";
import { clipboard } from "./clipboard";

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  Reflect.deleteProperty(document, "execCommand");
  document.body.replaceChildren();
});

describe("copying without the Clipboard API", () => {
  it("gives focus and the text selection back to where they were", async () => {
    vi.stubGlobal("navigator", { ...navigator, clipboard: undefined });
    // Browsers move focus to a text area that is selected; jsdom does not.
    vi.spyOn(HTMLTextAreaElement.prototype, "select").mockImplementation(function (this: HTMLTextAreaElement) {
      this.focus();
      this.setSelectionRange(0, this.value.length);
    });
    const copied: string[] = [];
    Object.defineProperty(document, "execCommand", {
      configurable: true,
      value: vi.fn(() => {
        const holder = document.activeElement as HTMLTextAreaElement;
        copied.push(holder.value.slice(holder.selectionStart, holder.selectionEnd));
        return true;
      }),
    });
    const input = document.createElement("input");
    input.value = "terminal input";
    document.body.append(input);
    input.focus();
    input.setSelectionRange(2, 5);

    await clipboard.writeText("selected output");

    expect(copied).toEqual(["selected output"]);
    expect(input).toHaveFocus();
    expect([input.selectionStart, input.selectionEnd]).toEqual([2, 5]);
  });
});
