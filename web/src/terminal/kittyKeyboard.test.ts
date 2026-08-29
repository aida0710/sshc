import { describe, expect, it, vi } from "vitest";
import { attachKittyKeyboardProtocol } from "./kittyKeyboard";

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
});
