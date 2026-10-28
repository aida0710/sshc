import { renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { apiClient } from "../api/client";
import { useSameDestination } from "./useSameDestination";

afterEach(() => vi.restoreAllMocks());

function answer(destinations: Record<string, { hostName: string; port: string; user: string }>) {
  vi.spyOn(apiClient, "read").mockImplementation(async (path: string) => {
    const base = Object.keys(destinations).find((prefix) => path.startsWith(prefix));
    if (base === undefined) throw new Error(`unexpected ${path}`);
    return { alias: "web", ...destinations[base] };
  });
}

describe("useSameDestination", () => {
  it("is true when the run target and this machine take the alias to the same host, port and user", async () => {
    answer({
      "/api/v1/terminal/": { hostName: "203.0.113.10", port: "22", user: "ops" },
      "/api/v1/engines/build-box/terminal/": { hostName: "203.0.113.10", port: "22", user: "ops" },
    });
    const { result } = renderHook(() => useSameDestination("web", "build-box"));
    await waitFor(() => expect(result.current).toBe(true));
  });

  it("is false when the run target's alias reaches another host", async () => {
    answer({
      "/api/v1/terminal/": { hostName: "203.0.113.10", port: "22", user: "ops" },
      "/api/v1/engines/build-box/terminal/": { hostName: "10.0.0.5", port: "22", user: "ops" },
    });
    const { result } = renderHook(() => useSameDestination("web", "build-box"));
    await waitFor(() => expect(apiClient.read).toHaveBeenCalledTimes(2));
    expect(result.current).toBe(false);
  });

  it("asks nothing for a session on this machine", () => {
    const read = vi.spyOn(apiClient, "read");
    const { result } = renderHook(() => useSameDestination("web", undefined));
    expect(result.current).toBe(false);
    expect(read).not.toHaveBeenCalled();
  });
});
