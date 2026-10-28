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

describe("copying text and HTML together", () => {
  it("puts both forms in one clipboard item", async () => {
    const written: Record<string, Blob>[] = [];
    vi.stubGlobal("ClipboardItem", class { constructor(readonly items: Record<string, Blob>) {} });
    vi.stubGlobal("navigator", { ...navigator, clipboard: { write: vi.fn(async (items: { items: Record<string, Blob> }[]) => { written.push(items[0]!.items); }) } });

    await clipboard.writeTextAndHTML("red", "<span style=\"color: #ff0000\">red</span>");

    expect(Object.keys(written[0] ?? {})).toEqual(["text/plain", "text/html"]);
    expect(await written[0]?.["text/html"]?.text()).toBe("<span style=\"color: #ff0000\">red</span>");
  });

  it("fills both forms in the copy event where the Clipboard API is missing", async () => {
    vi.stubGlobal("navigator", { ...navigator, clipboard: undefined });
    const formats: Record<string, string> = {};
    Object.defineProperty(document, "execCommand", {
      configurable: true,
      value: vi.fn(() => {
        const event = new Event("copy", { cancelable: true }) as ClipboardEvent;
        Object.defineProperty(event, "clipboardData", { value: { setData: (format: string, data: string) => { formats[format] = data; } } });
        document.dispatchEvent(event);
        return true;
      }),
    });

    await clipboard.writeTextAndHTML("red", "<b>red</b>");

    expect(formats).toEqual({ "text/plain": "red", "text/html": "<b>red</b>" });
  });
});
