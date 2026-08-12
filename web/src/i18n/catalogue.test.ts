import { describe, expect, it } from "vitest";
import { catalogueDifference } from "./catalogue";
import { en, messages } from "./messages";

describe("i18n catalogue coverage", () => {
  it("lists missing and extra keys relative to the English master", () => {
    expect(catalogueDifference(
      { alpha: "Alpha", beta: "Beta" },
      { beta: "ベータ", gamma: "ガンマ" },
    )).toEqual({ missing: ["alpha"], extra: ["gamma"] });
  });

  it("keeps every registered language aligned with the English master", () => {
    const problems = Object.entries(messages)
      .filter(([locale]) => locale !== "en")
      .map(([locale, catalogue]) => ({ locale, ...catalogueDifference(en, catalogue) }))
      .filter(({ missing, extra }) => missing.length > 0 || extra.length > 0);

    expect(problems).toEqual([]);
  });
});
