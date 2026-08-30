import { describe, expect, it, vi } from "vitest";
import { attachKittyKeyboardProtocol, encodeIntlYen } from "./kittyKeyboard";

function harness() {
  const handlers = new Map<string, (parameters: (number | number[])[]) => boolean>();
  const protocol = attachKittyKeyboardProtocol({
    registerCsiHandler: vi.fn((identifier, handler) => {
      handlers.set(identifier.prefix, handler);
      return { dispose: vi.fn() };
    }),
  });
  return { handlers, protocol };
}

describe("kitty keyboard protocol", () => {
  it("leaves modified Enter unchanged until the application enables the protocol", () => {
    const { handlers, protocol } = harness();
    const shiftEnter = new KeyboardEvent("keydown", { key: "Enter", shiftKey: true });
    expect(protocol.encode(shiftEnter)).toBeNull();

    handlers.get(">")?.([1]);
    expect(protocol.encode(shiftEnter)).toBe("\u001b[13;2u");
    expect(protocol.encode(new KeyboardEvent("keydown", { key: "Enter", ctrlKey: true }))).toBe("\u001b[13;5u");
  });

  it("restores the prior mode when the application pops its flags", () => {
    const { handlers, protocol } = harness();
    handlers.get(">")?.([1]);
    handlers.get(">")?.([0]);
    const shiftEnter = new KeyboardEvent("keydown", { key: "Enter", shiftKey: true });
    expect(protocol.encode(shiftEnter)).toBeNull();
    handlers.get("<")?.([1]);
    expect(protocol.encode(shiftEnter)).toBe("\u001b[13;2u");
    protocol.reset();
    expect(protocol.encode(shiftEnter)).toBeNull();
  });

  it("bounds remotely controlled state nesting and discards the oldest state", () => {
    const { handlers, protocol } = harness();
    const shiftEnter = new KeyboardEvent("keydown", { key: "Enter", shiftKey: true });
    handlers.get("=")?.([1]);

    for (let index = 0; index < 65; index += 1) handlers.get(">")?.([2]);
    handlers.get("<")?.([64]);
    expect(protocol.encode(shiftEnter)).toBe("\u001b[13;2u");

    handlers.get("<")?.([1]);
    expect(protocol.encode(shiftEnter)).toBeNull();
  });

  it("limits pop work to the number of retained states", () => {
    const { handlers, protocol } = harness();
    const shiftEnter = new KeyboardEvent("keydown", { key: "Enter", shiftKey: true });
    handlers.get(">")?.([1]);

    handlers.get("<")?.([Number.MAX_SAFE_INTEGER]);
    expect(protocol.encode(shiftEnter)).toBeNull();
  });

  it("encodes modified navigation, tab and printable keys after negotiation", () => {
    const { handlers, protocol } = harness();
    handlers.get(">")?.([1]);

    expect(protocol.encode(new KeyboardEvent("keydown", { key: "ArrowUp", ctrlKey: true }))).toBe("\u001b[1;5A");
    expect(protocol.encode(new KeyboardEvent("keydown", { key: "PageDown", shiftKey: true }))).toBe("\u001b[6;2~");
    expect(protocol.encode(new KeyboardEvent("keydown", { key: "Tab", shiftKey: true }))).toBe("\u001b[9;2u");
    expect(protocol.encode(new KeyboardEvent("keydown", { key: " ", ctrlKey: true }))).toBe("\u001b[32;5u");
    expect(protocol.encode(new KeyboardEvent("keydown", { key: "ArrowUp" }))).toBeNull();
  });
});

describe("JIS yen key", () => {
  it("maps IntlYen only when explicitly enabled and outside IME composition", () => {
    const yen = new KeyboardEvent("keydown", { code: "IntlYen", key: "¥" });
    expect(encodeIntlYen(yen, false)).toBeNull();
    expect(encodeIntlYen(yen, true)).toBe("\\");
    expect(encodeIntlYen(new KeyboardEvent("keydown", { code: "IntlYen", key: "|", shiftKey: true }), true)).toBe("|");
    expect(encodeIntlYen(new KeyboardEvent("keydown", { code: "IntlYen", key: "¥", ctrlKey: true }), true)).toBeNull();
  });
});
