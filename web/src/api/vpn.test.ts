import { afterEach, describe, expect, it, vi } from "vitest";
import { apiClient } from "./client";
import { vpnApi } from "./vpn";

afterEach(() => {
  apiClient.clear();
  vi.unstubAllGlobals();
});

describe("vpnApi", () => {
  it("asks the engine to open a route without a request body the API does not define", async () => {
    const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify({ available: true, profiles: [] }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }));
    vi.stubGlobal("fetch", fetcher);
    apiClient.setCSRF("c".repeat(43));

    await expect(vpnApi.startVPNSession("lab")).resolves.toEqual({ available: true, profiles: [] });

    const [path, request] = fetcher.mock.calls[0] as [string, RequestInit];
    expect(path).toBe("/api/v1/vpn/profiles/lab/session");
    expect(request.method).toBe("POST");
    expect(request.body).toBeUndefined();
  });
});
