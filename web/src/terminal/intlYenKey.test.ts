import { describe, expect, it } from "vitest";
import { encodeIntlYen } from "./intlYenKey";

describe("JIS yen key", () => {
  it("maps IntlYen only when explicitly enabled and outside IME composition", () => {
    const yen = new KeyboardEvent("keydown", { code: "IntlYen", key: "¥" });
    expect(encodeIntlYen(yen, false)).toBeNull();
    expect(encodeIntlYen(yen, true)).toBe("\\");
    expect(encodeIntlYen(new KeyboardEvent("keydown", { code: "IntlYen", key: "|", shiftKey: true }), true)).toBe("|");
    expect(encodeIntlYen(new KeyboardEvent("keydown", { code: "IntlYen", key: "¥", ctrlKey: true }), true)).toBeNull();
  });
});
