import { describe, expect, it } from "vitest";
import { eligibilityText } from "./eligibilityText";

describe("eligibilityText", () => {
  it("知っているコードを訳す", () => {
    expect(eligibilityText((key) => `<${key}>`, "password_authentication_off")).toBe(
      "<password.blocker.authenticationOff>",
    );
  });

  // **知らないコードを飲み込まない。** サーバーが規則を増やしたとき、画面から
  // 理由が消えるより、訳されていないコードがそのまま出る方がよい。
  it("知らないコードはそのまま返す", () => {
    expect(eligibilityText((key) => `<${key}>`, "some_new_rule")).toBe("some_new_rule");
  });
});
