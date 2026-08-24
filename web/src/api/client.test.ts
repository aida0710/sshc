import { afterEach, describe, expect, it, vi } from "vitest";
import { apiClient, whenSessionEnded } from "./client";

afterEach(() => {
  apiClient.clear();
  whenSessionEnded(null);
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

  it("renews a stale CSRF token and retries the rejected request once", async () => {
    const oldToken = "c".repeat(43);
    const freshToken = "d".repeat(43);
    const fetcher = vi.fn()
      .mockResolvedValueOnce(new Response(
        JSON.stringify({ code: "invalid_csrf", message: "request rejected" }),
        { status: 403, headers: { "Content-Type": "application/problem+json" } },
      ))
      .mockResolvedValueOnce(new Response(
        JSON.stringify({ csrfToken: freshToken }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ))
      .mockResolvedValueOnce(new Response(
        JSON.stringify({ value: "recovered" }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ));
    vi.stubGlobal("fetch", fetcher);
    apiClient.setCSRF(oldToken);

    await expect(apiClient.mutate<{ value: string }>("/api/v1/example", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ value: "kept" }),
    })).resolves.toEqual({ value: "recovered" });

    expect(fetcher).toHaveBeenCalledTimes(3);
    expect(fetcher.mock.calls[1]?.[0]).toBe("/api/v1/session/renew");
    const retry = fetcher.mock.calls[2]?.[1] as RequestInit;
    expect(new Headers(retry.headers).get("X-SSHC-CSRF")).toBe(freshToken);
    expect(retry.method).toBe("POST");
    expect(retry.body).toBe(JSON.stringify({ value: "kept" }));
  });

  it("shares one renewal between concurrently rejected requests", async () => {
    const oldToken = "c".repeat(43);
    const freshToken = "d".repeat(43);
    let releaseRenewal: ((response: Response) => void) | undefined;
    const renewalResponse = new Promise<Response>((resolve) => {
      releaseRenewal = resolve;
    });
    let renewals = 0;
    const fetcher = vi.fn((path: string, init?: RequestInit) => {
      if (path === "/api/v1/session/renew") {
        renewals += 1;
        return renewalResponse;
      }
      const token = new Headers(init?.headers).get("X-SSHC-CSRF");
      if (token === oldToken) {
        return Promise.resolve(new Response(
          JSON.stringify({ code: "invalid_csrf", message: "request rejected" }),
          { status: 403, headers: { "Content-Type": "application/problem+json" } },
        ));
      }
      return Promise.resolve(new Response(
        JSON.stringify({ path }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ));
    });
    vi.stubGlobal("fetch", fetcher);
    apiClient.setCSRF(oldToken);

    const first = apiClient.read("/api/v1/one");
    const second = apiClient.read("/api/v1/two");
    await vi.waitFor(() => expect(renewals).toBe(1));
    releaseRenewal?.(new Response(
      JSON.stringify({ csrfToken: freshToken }),
      { status: 200, headers: { "Content-Type": "application/json" } },
    ));

    await expect(Promise.all([first, second])).resolves.toEqual([
      { path: "/api/v1/one" },
      { path: "/api/v1/two" },
    ]);
    expect(renewals).toBe(1);
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

  it("announces the end of the session when CSRF renewal is rejected", async () => {
    const ended = vi.fn();
    whenSessionEnded(ended);
    const fetcher = vi.fn()
      .mockResolvedValueOnce(new Response(
        JSON.stringify({ code: "invalid_csrf", message: "request rejected" }),
        { status: 403, headers: { "Content-Type": "application/problem+json" } },
      ))
      .mockResolvedValueOnce(new Response(
        JSON.stringify({ code: "invalid_session", message: "request rejected" }),
        { status: 401, headers: { "Content-Type": "application/problem+json" } },
      ));
    vi.stubGlobal("fetch", fetcher);
    apiClient.setCSRF("c".repeat(43));

    await expect(apiClient.read("/api/v1/example")).rejects.toMatchObject({ code: "invalid_csrf" });

    expect(ended).toHaveBeenCalledTimes(1);
  });
});
