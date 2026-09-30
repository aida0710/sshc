import { describe, expect, it } from "vitest";
import type { MessageKey } from "./messages";
import { codeText, type CodeTextLookup } from "./codeText";

const t = (key: MessageKey) => `text of ${key}`;
const snippetLookup: CodeTextLookup = {
  messages: { snippet_not_found: "snippets.notFound" },
  fallback: "snippets.failed",
};

describe("codeText", () => {
  it("says a known code in the words of its message", () => {
    expect(codeText(t, "snippet_not_found", snippetLookup)).toBe("text of snippets.notFound");
  });

  it("falls back to the screen's general sentence for an unknown or missing code", () => {
    expect(codeText(t, "snippet_new_code", snippetLookup)).toBe("text of snippets.failed");
    expect(codeText(t, "", snippetLookup)).toBe("text of snippets.failed");
    expect(codeText(t, "constructor", snippetLookup)).toBe("text of snippets.failed");
  });
});
