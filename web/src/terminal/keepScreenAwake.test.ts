import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { localStorageKeys } from "../ui/browserStorageKeys";
import { loadKeepScreenAwake, saveKeepScreenAwake, useKeepScreenAwake } from "./keepScreenAwake";

describe("keepScreenAwake", () => {
  it("is off until this browser turns it on", () => {
    expect(loadKeepScreenAwake()).toBe(false);

    saveKeepScreenAwake(true);

    expect(loadKeepScreenAwake()).toBe(true);
    expect(window.localStorage.getItem(localStorageKeys.keepScreenAwake)).toBe("true");
  });

  it("tells every terminal of this tab when the choice changes", () => {
    const { result } = renderHook(() => useKeepScreenAwake());
    expect(result.current).toBe(false);

    act(() => saveKeepScreenAwake(true));

    expect(result.current).toBe(true);
  });
});
