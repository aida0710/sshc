import { describe, expect, it } from "vitest";
import { en, ja } from "../i18n/messages";
import {
  placeholders,
  snippetStatusLabelKey,
  snippetVariableTypeLabelKey,
  variablesFor,
} from "./SnippetsPanel";

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

  it("maps variable types and execution statuses to English and Japanese labels", () => {
    expect(en[snippetVariableTypeLabelKey("secret")]).toBe("Secret");
    expect(ja[snippetVariableTypeLabelKey("secret")]).toBe("シークレット");
    expect(en[snippetVariableTypeLabelKey("internal_future_type")]).toBe("Unknown type");
    expect(ja[snippetVariableTypeLabelKey("internal_future_type")]).toBe("不明な型");
    expect(en[snippetStatusLabelKey("succeeded")]).toBe("Succeeded");
    expect(ja[snippetStatusLabelKey("succeeded")]).toBe("成功");
    expect(en[snippetStatusLabelKey("internal_future_code")]).toBe("Status unavailable");
    expect(ja[snippetStatusLabelKey("internal_future_code")]).toBe("状態を確認できません");
  });
});
