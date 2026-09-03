import { describe, expect, it } from "vitest";
import { applyTerminalRuntimeOptions } from "./runtimeOptions";

describe("live terminal options", () => {
  it("applies settings to an existing terminal and requests a refit for font changes", () => {
    const options = { cursorBlink: false, fontSize: 13, scrollback: 5_000 };

    const result = applyTerminalRuntimeOptions(options, {
      cursorBlink: true,
      fontSize: 16,
      scrollback: 20_000,
    });

    expect(options).toEqual({ cursorBlink: true, fontSize: 16, scrollback: 20_000 });
    expect(result.refit).toBe(true);
  });

  it("does not request a geometry change when only lifecycle or scrollback changes", () => {
    const options = { cursorBlink: true, fontSize: 15, scrollback: 5_000 };

    const result = applyTerminalRuntimeOptions(options, {
      cursorBlink: false,
      fontSize: 15,
      scrollback: 1_000,
    });

    expect(options).toEqual({ cursorBlink: false, fontSize: 15, scrollback: 1_000 });
    expect(result.refit).toBe(false);
  });
});
