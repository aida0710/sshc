import { describe, expect, it } from "vitest";
import { newIdentifier, unguessableToken } from "./randomIdentifier";

// 32 lowercase hex characters fit every pattern the values are checked
// against: SFTP transfer IDs and shortcut preset IDs in the engine, and the
// save request ID in the Android bridge.
const thirtyTwoHex = /^[0-9a-f]{32}$/;

describe("randomIdentifier", () => {
  it("makes 128 random bits as hex, and a new value each time", () => {
    for (const make of [unguessableToken, newIdentifier]) {
      const first = make();
      const second = make();
      expect(first).toMatch(thirtyTwoHex);
      expect(second).toMatch(thirtyTwoHex);
      expect(first).not.toBe(second);
    }
  });

  it("does not need crypto.randomUUID, which a page outside a secure context lacks", () => {
    const randomUUID = Object.getOwnPropertyDescriptor(crypto, "randomUUID");
    Object.defineProperty(crypto, "randomUUID", { value: undefined, configurable: true });
    try {
      expect(newIdentifier()).toMatch(thirtyTwoHex);
      expect(unguessableToken()).toMatch(thirtyTwoHex);
    } finally {
      if (randomUUID === undefined) Reflect.deleteProperty(crypto, "randomUUID");
      else Object.defineProperty(crypto, "randomUUID", randomUUID);
    }
  });
});
