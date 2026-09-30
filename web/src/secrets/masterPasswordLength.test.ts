import { describe, expect, it } from "vitest";
import { isAcceptedButShort, masterPasswordLength, meetsMasterPasswordMinimum } from "./masterPasswordLength";

describe("master password length", () => {
  it("counts an emoji as one character, as the engine does", () => {
    expect(masterPasswordLength("🔑🔑")).toBe(2);
    expect(meetsMasterPasswordMinimum("🔑🔑", 4)).toBe(false);
    expect(meetsMasterPasswordMinimum("🔑🔑🔑🔑", 4)).toBe(true);
  });

  it("follows the minimum the engine reports", () => {
    expect(meetsMasterPasswordMinimum("abcde", 6)).toBe(false);
    expect(meetsMasterPasswordMinimum("abcdef", 6)).toBe(true);
  });

  it("accepts nothing while the engine's minimum is unknown", () => {
    expect(meetsMasterPasswordMinimum("a long enough password", undefined)).toBe(false);
  });

  it("calls a password short only between the minimum and the recommended length", () => {
    expect(isAcceptedButShort("abc", 4)).toBe(false);
    expect(isAcceptedButShort("abcd", 4)).toBe(true);
    expect(isAcceptedButShort("abcdefghijk", 4)).toBe(true);
    expect(isAcceptedButShort("abcdefghijkl", 4)).toBe(false);
  });
});
