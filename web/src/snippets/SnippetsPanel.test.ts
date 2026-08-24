import { describe, expect, it } from "vitest";
import { placeholders, variablesFor } from "./SnippetsPanel";

describe("snippet variables", () => {
  it("finds placeholders once in command order", () => {
    expect(placeholders("deploy {{environment}} {{count}} {{environment}} {{not-valid}}"))
      .toEqual(["environment", "count"]);
  });

  it("preserves explicitly selected types while removing unused variables", () => {
    expect(variablesFor("echo {{count}} {{token}}", [
      { name: "count", type: "integer", required: true },
      { name: "old", type: "boolean", required: true },
    ])).toEqual([
      { name: "count", type: "integer", required: true },
      { name: "token", type: "string", required: true },
    ]);
  });
});
