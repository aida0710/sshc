import { afterEach, describe, expect, it, vi } from "vitest";
import { apiClient, whenRequestFailed, whenSessionEnded } from "./client";

afterEach(() => {
  apiClient.clear();
  whenSessionEnded(null);
  whenRequestFailed(null);
  vi.unstubAllGlobals();
});

describe("apiClient", () => {
  it("returns only a runtime-valid health response", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ status: "ok", version: "0.1.0" }),
      { status: 200, headers: { "Content-Type": "application/json" } },
    )));

    await expect(apiClient.health()).resolves.toEqual({ status: "ok", version: "0.1.0" });
  });

  it("rejects malformed health payloads", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ status: "ok", version: "" }),
      { status: 200, headers: { "Content-Type": "application/json" } },
    )));

    await expect(apiClient.health()).rejects.toThrow("invalid_health_response");
  });

  it("adds the module-memory CSRF token to mutations", async () => {
    const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify({ ok: true }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }));
    vi.stubGlobal("fetch", fetcher);
    apiClient.setCSRF("c".repeat(43));

    await expect(apiClient.mutate<{ ok: boolean }>("/api/v1/example", { method: "POST" }))
      .resolves.toEqual({ ok: true });

    const request = fetcher.mock.calls[0]?.[1] as RequestInit;
    expect(new Headers(request.headers).get("X-SSHC-CSRF")).toBe("c".repeat(43));
    expect(request.credentials).toBe("same-origin");
  });

  it("rejects mutations before a CSRF token is set", async () => {
    await expect(apiClient.mutate("/api/v1/example", { method: "POST" })).rejects.toThrow("csrf_unavailable");
  });

  it.each(["https://evil.example/api/v1/example", "//evil.example/api/v1/example"])(
    "rejects a cross-origin mutation without calling fetch: %s",
    async (path) => {
      const fetcher = vi.fn();
      vi.stubGlobal("fetch", fetcher);
      apiClient.setCSRF("c".repeat(43));

      await expect(apiClient.mutate(path, { method: "POST" })).rejects.toThrow("cross_origin_api_mutation");

      expect(fetcher).not.toHaveBeenCalled();
    },
  );

  it("ends the session on a rejected CSRF token without asking the engine to renew it", async () => {
    const ended = vi.fn();
    whenSessionEnded(ended);
    const fetcher = vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ code: "invalid_csrf", message: "request rejected" }),
      { status: 403, headers: { "Content-Type": "application/problem+json" } },
    ));
    vi.stubGlobal("fetch", fetcher);
    apiClient.setCSRF("c".repeat(43));

    await expect(apiClient.mutate("/api/v1/example", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ value: "kept" }),
    })).rejects.toMatchObject({ code: "invalid_csrf" });

    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(fetcher.mock.calls.map(([path]) => path)).not.toContain("/api/v1/session/renew");
    expect(ended).toHaveBeenCalledTimes(1);
    await expect(apiClient.read("/api/v1/example")).rejects.toThrow("csrf_unavailable");
  });

  it("announces the end of the session once when concurrent requests are rejected for CSRF", async () => {
    const ended = vi.fn();
    whenSessionEnded(ended);
    const fetcher = vi.fn(() => Promise.resolve(new Response(
      JSON.stringify({ code: "invalid_csrf", message: "request rejected" }),
      { status: 403, headers: { "Content-Type": "application/problem+json" } },
    )));
    vi.stubGlobal("fetch", fetcher);
    apiClient.setCSRF("c".repeat(43));

    const results = await Promise.allSettled([
      apiClient.read("/api/v1/one"),
      apiClient.read("/api/v1/two"),
    ]);

    expect(results.map((result) => result.status)).toEqual(["rejected", "rejected"]);
    expect(fetcher).toHaveBeenCalledTimes(2);
    expect(ended).toHaveBeenCalledTimes(1);
  });

  it("announces an invalid session and clears its token", async () => {
    const ended = vi.fn();
    whenSessionEnded(ended);
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ code: "invalid_session", message: "request rejected" }),
      { status: 401, headers: { "Content-Type": "application/problem+json" } },
    )));
    apiClient.setCSRF("c".repeat(43));

    await expect(apiClient.read("/api/v1/example")).rejects.toMatchObject({ code: "invalid_session" });

    expect(ended).toHaveBeenCalledTimes(1);
    await expect(apiClient.read("/api/v1/example")).rejects.toThrow("csrf_unavailable");
  });

  it("reports the final API failure without exposing query parameters", async () => {
    const diagnostic = vi.fn();
    whenRequestFailed(diagnostic);
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ code: "sftp_failed", message: "request rejected", detail: "connection closed" }),
      { status: 502, headers: { "Content-Type": "application/problem+json" } },
    )));
    apiClient.setCSRF("c".repeat(43));

    await expect(apiClient.read("/api/v1/sftp/files?alias=private-host&path=%2Fsecret"))
      .rejects.toMatchObject({ code: "sftp_failed" });

    expect(diagnostic).toHaveBeenCalledWith({
      code: "sftp_failed",
      status: 502,
      method: "GET",
      path: "/api/v1/sftp/files",
      detail: "connection closed",
    });
  });

  it("leaves SFTP connection failures to the SFTP panel", async () => {
    const diagnostic = vi.fn();
    whenRequestFailed(diagnostic);
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ code: "sftp_failed", message: "request rejected" }),
      { status: 502, headers: { "Content-Type": "application/problem+json" } },
    )));
    apiClient.setCSRF("c".repeat(43));

    await expect(apiClient.read("/api/v1/sftp/miyabi/entries?path=%2F", {
      locallyHandledCodes: ["sftp_failed"],
    }))
      .rejects.toMatchObject({ code: "sftp_failed" });

    expect(diagnostic).not.toHaveBeenCalled();
  });

  it("leaves locally handled mutation failures to the operation panel", async () => {
    const diagnostic = vi.fn();
    whenRequestFailed(diagnostic);
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ code: "bucket_unreachable", message: "request rejected" }),
      { status: 502, headers: { "Content-Type": "application/problem+json" } },
    )));
    apiClient.setCSRF("c".repeat(43));

    await expect(apiClient.mutate("/api/v1/sync/pull", {
      method: "POST",
      body: JSON.stringify({ apply: true, resolve: "remote" }),
    }, {
      locallyHandledCodes: ["bucket_unreachable"],
    })).rejects.toMatchObject({ code: "bucket_unreachable" });

    expect(diagnostic).not.toHaveBeenCalled();
  });

  it("accepts a successful OpenAPI no-content mutation without parsing JSON", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(null, { status: 204 })));
    apiClient.setCSRF("c".repeat(43));

    await expect(apiClient.mutate<void>("/api/v1/sftp/transfers/finished", { method: "DELETE" }))
      .resolves.toBeUndefined();
  });

  it("reports network failure using fixed safe diagnostics", async () => {
    const diagnostic = vi.fn();
    whenRequestFailed(diagnostic);
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new TypeError("fetch failed for secret URL")));
    apiClient.setCSRF("c".repeat(43));

    await expect(apiClient.mutate("/api/v1/sync/push?access_key=secret", {
      method: "POST",
      body: JSON.stringify({ secret: "not reported" }),
    })).rejects.toThrow("fetch failed");

    expect(diagnostic).toHaveBeenCalledWith({
      code: "network_request_failed",
      status: 0,
      method: "POST",
      path: "/api/v1/sync/push",
    });
  });

  it("reports failed raw responses used by streaming APIs", async () => {
    const diagnostic = vi.fn();
    whenRequestFailed(diagnostic);
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ code: "download_failed", message: "request rejected" }),
      { status: 502, headers: { "Content-Type": "application/problem+json" } },
    )));
    apiClient.setCSRF("c".repeat(43));

    const response = await apiClient.send("/api/v1/sftp/download?path=%2Fprivate", { method: "GET" });

    expect(response.status).toBe(502);
    expect(diagnostic).toHaveBeenCalledWith({
      code: "download_failed",
      status: 502,
      method: "GET",
      path: "/api/v1/sftp/download",
    });
  });

  it("does not report session lifecycle responses as operation failures", async () => {
    const diagnostic = vi.fn();
    whenRequestFailed(diagnostic);
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ code: "invalid_session", message: "request rejected" }),
      { status: 401, headers: { "Content-Type": "application/problem+json" } },
    )));
    apiClient.setCSRF("c".repeat(43));

    await expect(apiClient.read("/api/v1/example")).rejects.toMatchObject({ code: "invalid_session" });
    expect(diagnostic).not.toHaveBeenCalled();
  });

  it.each(["workspace_busy", "workspace_pending_transaction"])(
    "reports a workspace refusal no screen explains even though it is a 409 (%s)",
    async (code) => {
      const diagnostic = vi.fn();
      whenRequestFailed(diagnostic);
      vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(
        JSON.stringify({ code, message: "request rejected", detail: "fixed detail" }),
        { status: 409, headers: { "Content-Type": "application/problem+json" } },
      )));
      apiClient.setCSRF("c".repeat(43));

      await expect(apiClient.mutate("/api/v1/config/save", { method: "POST", body: "{}" }))
        .rejects.toMatchObject({ code });

      expect(diagnostic).toHaveBeenCalledWith({
        code, status: 409, method: "POST", path: "/api/v1/config/save", detail: "fixed detail",
      });
    },
  );

  it.each([
    [409, "sync_remote_moved"],
    [502, "update_check_failed"],
  ])("leaves expected or background failures to their local UI (%i %s)", async (status, code) => {
    const diagnostic = vi.fn();
    whenRequestFailed(diagnostic);
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ code, message: "request rejected" }),
      { status, headers: { "Content-Type": "application/problem+json" } },
    )));
    apiClient.setCSRF("c".repeat(43));

    await expect(apiClient.read("/api/v1/example")).rejects.toMatchObject({ code });
    expect(diagnostic).not.toHaveBeenCalled();
  });
});
