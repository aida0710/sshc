import { renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useRequestGeneration } from "./useRequestGeneration";

describe("useRequestGeneration", () => {
  it("lets only the latest begun request apply its answer", () => {
    const { result } = renderHook(() => useRequestGeneration());
    const first = result.current.begin();
    const second = result.current.begin();
    expect(first()).toBe(false);
    expect(second()).toBe(true);
  });

  it("keeps an observed read current until something begins or is retired", () => {
    const { result } = renderHook(() => useRequestGeneration());
    const read = result.current.observe();
    const laterRead = result.current.observe();
    expect(read()).toBe(true);
    expect(laterRead()).toBe(true);
    result.current.begin();
    expect(read()).toBe(false);
    const afterWrite = result.current.observe();
    result.current.retire();
    expect(afterWrite()).toBe(false);
  });

  it("stays the same object across renders so it can sit in dependencies", () => {
    const { result, rerender } = renderHook(() => useRequestGeneration());
    const first = result.current;
    rerender();
    expect(result.current).toBe(first);
  });
});
