import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useOfferedDialog } from "./useOfferedDialog";

describe("useOfferedDialog", () => {
  it("does not reopen a declined offer until that target is chosen again", () => {
    const { result } = renderHook(() => useOfferedDialog());
    act(() => result.current.offer("r100"));
    expect(result.current.openFor).toBe("r100");
    act(() => result.current.close());
    act(() => result.current.offer("r100"));
    expect(result.current.openFor).toBeNull();
    act(() => result.current.allowOffer("r100"));
    act(() => result.current.offer("r100"));
    expect(result.current.openFor).toBe("r100");
  });

  it("lets the user reopen a declined dialog and offers other targets independently", () => {
    const { result } = renderHook(() => useOfferedDialog());
    act(() => result.current.offer("r100"));
    act(() => result.current.close());
    act(() => result.current.offer("gpu-box"));
    expect(result.current.openFor).toBe("gpu-box");
    act(() => result.current.close());
    act(() => result.current.ask("r100"));
    expect(result.current.openFor).toBe("r100");
    act(() => result.current.finish());
    expect(result.current.openFor).toBeNull();
    act(() => result.current.offer("gpu-box"));
    expect(result.current.openFor).toBeNull();
    act(() => result.current.offer("r100"));
    expect(result.current.openFor).toBe("r100");
  });
});
