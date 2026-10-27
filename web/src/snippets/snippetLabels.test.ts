import { describe, expect, it } from "vitest";
import { en, ja } from "../i18n/messages";
import { snippetProblemLabelKey, snippetStatusLabelKey, snippetVariableTypeLabelKey } from "./snippetLabels";

describe("snippet labels", () => {
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

  it("explains a host stopped by a time limit separately from a cancellation", () => {
    const timedOut = snippetProblemLabelKey("timed_out");
    const cancelled = snippetProblemLabelKey("cancelled");
    expect(timedOut && ja[timedOut]).toBe("タイムアウトしました");
    expect(timedOut && en[timedOut]).toBe("Timed out");
    expect(cancelled).not.toBe(timedOut);
    expect(snippetProblemLabelKey("internal_future_code")).toBeUndefined();
    expect(snippetProblemLabelKey(undefined)).toBeUndefined();
  });
});
