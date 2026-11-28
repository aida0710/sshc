import { describe, expect, it } from "vitest";
import { withSnippetInput } from "./snippetInputs";

describe("withSnippetInput", () => {
  it("drops the key when a boolean set to true is put back to its default", () => {
    const chosen = withSnippetInput({}, "follow", "true");

    expect(withSnippetInput(chosen, "follow", "")).toEqual({});
  });

  it("drops only that variable's key when its number is cleared", () => {
    expect(withSnippetInput({ lines: "5", unit: "sshd" }, "lines", "")).toEqual({ unit: "sshd" });
  });
});
