import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { apiClient } from "./client";
import { appearanceApi } from "./appearance";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

beforeEach(() => {
  apiClient.setCSRF("c".repeat(43));
});

afterEach(() => {
  apiClient.clear();
  vi.unstubAllGlobals();
});

describe("appearanceApi", () => {
  it("reads the accent from the metadata and treats a missing section as nothing chosen", async () => {
    vi.stubGlobal("fetch", vi.fn()
      .mockResolvedValueOnce(jsonResponse({ schemaVersion: 9, appearance: { accent: "pink" } }))
      .mockResolvedValueOnce(jsonResponse({ schemaVersion: 9 })));

    await expect(appearanceApi.appearanceSettings()).resolves.toEqual({ accent: "pink" });
    await expect(appearanceApi.appearanceSettings()).resolves.toEqual({});
  });

  it("writes the accent to its own metadata endpoint", async () => {
    const fetch = vi.fn().mockResolvedValue(jsonResponse({ transactionId: "t", written: [] }));
    vi.stubGlobal("fetch", fetch);

    await appearanceApi.setAppearanceSettings({ accent: "sky" });

    const [url, init] = fetch.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/api/v1/metadata/appearance");
    expect(init.method).toBe("PUT");
    expect(JSON.parse(init.body as string)).toEqual({ accent: "sky" });
  });
});
