import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { apiClient } from "./client";
import { recentConnectionsApi } from "./recentConnections";

const csrfToken = "c".repeat(43);


function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

beforeEach(() => {
  apiClient.setCSRF(csrfToken);
});

afterEach(() => {
  apiClient.clear();
  vi.unstubAllGlobals();
});

describe("recentConnectionsApi recent connections", () => {
  const connection = {
    alias: "bastion",
    hostName: "bastion.example",
    user: "deploy",
    port: "2202",
    lastConnectedAt: "2026-08-24T15:30:00Z",
  };

  it("reads the current target without persisting anything in browser storage", async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValue(jsonResponse({ connections: [connection] }));
    vi.stubGlobal("fetch", fetcher);

    await expect(recentConnectionsApi.recentConnections()).resolves.toEqual({
      connections: [connection],
    });
    expect(fetcher).toHaveBeenCalledWith(
      "/api/v1/connections/recent",
      expect.any(Object),
    );
    expect(window.localStorage.length).toBe(0);
    expect(window.sessionStorage.length).toBe(0);
  });

  it.each([
    { connections: {} },
    { connections: [{ ...connection, port: 22 }] },
    { connections: [{ ...connection, lastConnectedAt: false }] },
  ])("rejects malformed recent connection data %#", async (body) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(body)));
    await expect(recentConnectionsApi.recentConnections()).rejects.toThrow(
      "invalid_response",
    );
  });
});
