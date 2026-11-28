import { describe, expect, it } from "vitest";
import { withSnippetInput } from "./snippetInputs";

describe("withSnippetInput", () => {
  it("真偽値を true にしてから［既定値を使用］に戻すと、キーが消える", () => {
    const chosen = withSnippetInput({}, "follow", "true");

    expect(withSnippetInput(chosen, "follow", "")).toEqual({});
  });

  it("整数の欄を消すと、その変数のキーだけが消える", () => {
    expect(withSnippetInput({ lines: "5", unit: "sshd" }, "lines", "")).toEqual({ unit: "sshd" });
  });
});
