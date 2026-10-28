import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useLoad } from "./useLoad";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej; });
  return { promise, resolve, reject };
}

describe("useLoad", () => {
  it("reads once, then reports the value", async () => {
    const answer = deferred<string>();
    const load = () => answer.promise;
    const { result } = renderHook(() => useLoad(load));
    expect(result.current.state).toBe("loading");
    await act(async () => { answer.resolve("ready"); });
    expect(result.current).toMatchObject({ state: "loaded", value: "ready" });
  });

  it("drops a slow answer for the previous target", async () => {
    const first = deferred<string>();
    const second = deferred<string>();
    const { result, rerender } = renderHook(({ load }) => useLoad(load), { initialProps: { load: () => first.promise } });
    rerender({ load: () => second.promise });
    await act(async () => { first.resolve("old"); });
    expect(result.current.state).toBe("loading");
    await act(async () => { second.resolve("new"); });
    expect(result.current).toMatchObject({ state: "loaded", value: "new" });
  });

  it("reports a failure and reads again on retry", async () => {
    let calls = 0;
    const load = () => (++calls === 1 ? Promise.reject(new Error("offline")) : Promise.resolve("back"));
    const { result } = renderHook(() => useLoad(load));
    await waitFor(() => expect(result.current.state).toBe("failed"));
    act(() => result.current.retry());
    await waitFor(() => expect(result.current).toMatchObject({ state: "loaded", value: "back" }));
  });
});
