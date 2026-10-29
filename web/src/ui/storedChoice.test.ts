import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { createStoredChoice, useStoredChoice } from "./storedChoice";

const key = "sshc.test.choice.v1";
const size = () => createStoredChoice({ key, choices: ["small", "large"], fallback: "small" });

describe("createStoredChoice", () => {
  it("reads the fallback until this browser saves a choice, and for a value it does not know", () => {
    const choice = size();
    expect(choice.load()).toBe("small");

    choice.save("large");
    expect(choice.load()).toBe("large");
    expect(window.localStorage.getItem(key)).toBe("large");

    window.localStorage.setItem(key, "huge");
    expect(choice.load()).toBe("small");
  });

  it("tells every reader in this tab when the choice changes", () => {
    const choice = size();
    const { result } = renderHook(() => useStoredChoice(choice));
    expect(result.current).toBe("small");

    act(() => choice.save("large"));

    expect(result.current).toBe("large");
  });

  it("follows a change made in another tab", () => {
    const choice = size();
    const { result } = renderHook(() => useStoredChoice(choice));

    act(() => {
      window.localStorage.setItem(key, "large");
      window.dispatchEvent(new StorageEvent("storage", { key }));
    });

    expect(result.current).toBe("large");
  });
});
