import { afterEach, describe, expect, it, vi } from "vitest";
import { apiClient, whenRequestFailed } from "./client";
import { remoteEnginesApi } from "./remoteEngines";

afterEach(() => {
  apiClient.clear();
  whenRequestFailed(null);
  vi.unstubAllGlobals();
});

describe("remoteEnginesApi.list", () => {
  it("このマシンのエンジンに届かないときは、共通の通知を出さずに失敗だけを返す", async () => {
    const diagnostic = vi.fn();
    whenRequestFailed(diagnostic);
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new TypeError("fetch failed")));
    apiClient.setCSRF("c".repeat(43));

    await expect(remoteEnginesApi.list()).rejects.toThrow("fetch failed");

    expect(diagnostic).not.toHaveBeenCalled();
  });

  it("エンジンが一覧を返せないときは、共通の通知を出す", async () => {
    const diagnostic = vi.fn();
    whenRequestFailed(diagnostic);
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ code: "internal_error", message: "request failed" }),
      { status: 500, headers: { "Content-Type": "application/problem+json" } },
    )));
    apiClient.setCSRF("c".repeat(43));

    await expect(remoteEnginesApi.list()).rejects.toMatchObject({ code: "internal_error" });

    expect(diagnostic).toHaveBeenCalledWith({ code: "internal_error", status: 500, method: "GET", path: "/api/v1/engines" });
  });
});
