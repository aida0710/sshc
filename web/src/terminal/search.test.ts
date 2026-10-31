import { describe, expect, it } from "vitest";
import { validSearchPattern } from "./search";

describe("terminal scrollback search", () => {
  it("rejects an invalid regular expression before it reaches xterm", () => {
    expect(validSearchPattern("[", { caseSensitive: false, regex: true })).toBe(false);
    expect(validSearchPattern("[a-z]+", { caseSensitive: true, regex: true })).toBe(true);
    expect(validSearchPattern("[", { caseSensitive: false, regex: false })).toBe(true);
  });
});
