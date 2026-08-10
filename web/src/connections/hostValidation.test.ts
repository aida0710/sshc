import { describe, expect, it } from "vitest";
import { validHostNameInput } from "./hostValidation";

describe("validHostNameInput", () => {
  it.each(["example.com", "203.0.113.10", "2001:db8::1", "::1", "2001:db8::", "::ffff:192.0.2.1"])(
    "accepts %s",
    (value) => expect(validHostNameInput(value)).toBe(true),
  );

  it.each(["", "bad host", "[::1]", "2001:::1", "1::2::3", "::ffff:999.0.2.1"])(
    "rejects %s",
    (value) => expect(validHostNameInput(value)).toBe(false),
  );
});
