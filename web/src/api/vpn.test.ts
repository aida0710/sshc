import { afterEach, describe, expect, it, vi } from "vitest";
import { apiClient } from "./client";
import { VPN_REVEAL_ACTION_KIND, vpnApi } from "./vpn";

afterEach(() => {
  apiClient.clear();
  vi.unstubAllGlobals();
});

describe("vpnApi", () => {
  // 経路の起動と切断は同じ場所へ送り、メソッドだけが違う。openapi はどちらにも本文を
  // 定めておらず、JSON を付けると送る前の検査（validateAPIRequest）が断る。
  it.each([
    { operation: "startVPNRoute", method: "POST" },
    { operation: "disconnectVPNRoute", method: "DELETE" },
  ] as const)("sends $operation as $method to the route without a request body the API does not define", async ({ operation, method }) => {
    const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify({ available: true, checking: false, profiles: [] }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }));
    vi.stubGlobal("fetch", fetcher);
    apiClient.setCSRF("c".repeat(43));

    await expect(vpnApi[operation]("lab")).resolves.toEqual({ available: true, checking: false, profiles: [] });

    const [path, request] = fetcher.mock.calls[0] as [string, RequestInit];
    expect(path).toBe("/api/v1/vpn/profiles/lab/route");
    expect(request.method).toBe(method);
    expect(request.body).toBeUndefined();
  });

  it("takes the stored secrets out with a one-time confirmation for that profile", async () => {
    const fetcher = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ token: "t".repeat(43), expiresAt: "2026-09-29T00:00:00Z" }), {
        status: 201,
        headers: { "Content-Type": "application/json" },
      }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ openconnectPassword: "a password" }), {
        status: 200,
        headers: { "Content-Type": "application/json", "Cache-Control": "no-store" },
      }));
    vi.stubGlobal("fetch", fetcher);
    apiClient.setCSRF("c".repeat(43));

    await expect(vpnApi.revealVPNSecrets("office")).resolves.toEqual({ openconnectPassword: "a password" });

    const [actionPath, actionRequest] = fetcher.mock.calls[0] as [string, RequestInit];
    expect(actionPath).toBe("/api/v1/actions");
    expect(JSON.parse(actionRequest.body as string)).toEqual({ kind: VPN_REVEAL_ACTION_KIND, target: "office" });
    const [path, request] = fetcher.mock.calls[1] as [string, RequestInit];
    expect(path).toBe("/api/v1/vpn/profiles/office/reveal");
    expect(request.method).toBe("POST");
    expect(new Headers(request.headers).get("X-SSHC-Action")).toBe("t".repeat(43));
  });
});
