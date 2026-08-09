import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { apiClient, ApiError } from "./client";
import { configApi, type CreateConnectionRequest } from "./config";

const overviewPayload = {
  entry: { path: "config", absolute: "/home/tester/.ssh/config" },
  files: [{ file: { path: "config", absolute: "/home/tester/.ssh/config" }, editable: true, loads: 1 }],
  hosts: [{
    identity: { path: "config", alias: "bastion" },
    file: { path: "config", absolute: "/home/tester/.ssh/config" },
    line: 1,
    patterns: ["bastion"],
    editable: true,
  }],
  metadata: { schemaVersion: 1 },
  groups: [],
  diagnostics: [],
  notices: [],
};

afterEach(() => {
  apiClient.clear();
  vi.unstubAllGlobals();
});

describe("configApi", () => {
  // 読み取りは今やトークンを運ぶため、これらはすべてセッションを必要とする。
  // クッキーはポートにスコープされないが、トークンはされる。
  beforeEach(() => {
    apiClient.setCSRF("t".repeat(43));
  });

  it("returns a runtime-validated overview", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(
      JSON.stringify(overviewPayload),
      { status: 200, headers: { "Content-Type": "application/json" } },
    )));

    const overview = await configApi.overview();

    expect(overview.hosts[0]?.identity.alias).toBe("bastion");
  });

  it("rejects an overview whose shape does not match the contract", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ entry: {}, files: [], hosts: "not-an-array", metadata: {}, diagnostics: [], notices: [] }),
      { status: 200, headers: { "Content-Type": "application/json" } },
    )));

    await expect(configApi.overview()).rejects.toThrow("invalid_response");
  });

  it("escapes query parameters instead of concatenating them", async () => {
    const fetcher = vi.fn().mockResolvedValue(new Response(
      JSON.stringify({
        form: { entry: overviewPayload.hosts[0], fields: [], raw: "" },
        metadata: { identity: { path: "config", alias: "a b" } },
        effective: { alias: "a b", approximate: true, entries: [] },
        file: {
          file: { path: "config", absolute: "/home/tester/.ssh/config" },
          contents: "", digest: "", editable: true, exists: true,
        },
      }),
      { status: 200, headers: { "Content-Type": "application/json" } },
    ));
    vi.stubGlobal("fetch", fetcher);

    await configApi.host("conf.d/10 home.conf", "a b");

    expect(fetcher.mock.calls[0]?.[0]).toBe("/api/v1/config/host?path=conf.d%2F10+home.conf&alias=a+b");
  });

  it("never persists the CSRF token or configuration text to storage or a global", async () => {
    const setItem = vi.spyOn(Storage.prototype, "setItem");
    const token = "c".repeat(43);
    const secret = "Host bastion\n\tHostName 203.0.113.10\n";
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(
      JSON.stringify({
        transactionId: "t1",
        written: ["config"],
        preview: { operation: "config.file_raw", diffs: [] },
      }),
      { status: 200, headers: { "Content-Type": "application/json" } },
    )));
    apiClient.setCSRF(token);

    await configApi.save({ kind: "file_raw", path: "config", base: secret, raw: secret });

    expect(setItem).not.toHaveBeenCalled();
    expect(window.localStorage).toHaveLength(0);
    expect(window.sessionStorage).toHaveLength(0);

    const holder = window as unknown as Record<string, unknown>;
    for (const key of Object.keys(holder)) {
      const value = holder[key];
      if (typeof value !== "string") continue;
      expect(value).not.toContain(token);
      expect(value).not.toContain("203.0.113.10");
    }
    setItem.mockRestore();
  });

  it("surfaces the problem code and conflict report of a rejected save", async () => {
    const conflict = {
      path: "config",
      externalChange: [{ op: "insert", text: "Host other", newLine: 4 }],
      localChange: [{ op: "delete", text: "\tPort 22", oldLine: 3 }],
    };
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ code: "config_conflict", message: "request rejected", path: "config", conflict }),
      { status: 409, headers: { "Content-Type": "application/problem+json" } },
    )));
    apiClient.setCSRF("c".repeat(43));

    const failure = await configApi.save({ kind: "file_raw", path: "config", raw: "Host a\n" }).catch((error: unknown) => error);

    expect(failure).toBeInstanceOf(ApiError);
    const apiError = failure as ApiError;
    expect(apiError.code).toBe("config_conflict");
    expect(apiError.status).toBe(409);
    expect(apiError.problem?.conflict?.externalChange).toHaveLength(1);
  });

  it("creates a connection from generated request types and accepts 201", async () => {
    const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      transactionId: "tx-create",
      identity: { path: "connections/home/edge.conf", alias: "edge" },
      preview: { operation: "connection.create", diffs: [] },
    }), { status: 201, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetcher);
    const request: CreateConnectionRequest = {
      alias: "edge",
      group: "home",
      hostName: "edge.example",
      user: "ops",
      authentication: { kind: "identity_file", keyId: "0123456789abcdef0123456789abcdef" },
    };

    const created = await configApi.createConnection(request);

    expect(created.identity).toEqual({ path: "connections/home/edge.conf", alias: "edge" });
    const [path, init] = fetcher.mock.calls[0] as [string, RequestInit];
    expect(path).toBe("/api/v1/connections");
    expect(init.method).toBe("POST");
    expect(JSON.parse(String(init.body))).toEqual(request);
  });

  it("propagates a connection creation problem", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
      code: "alias_already_declared", message: "request rejected",
    }), { status: 409, headers: { "Content-Type": "application/problem+json" } })));

    await expect(configApi.createConnection({
      alias: "bastion",
      hostName: "duplicate.example",
      authentication: { kind: "dedicated_password", password: "secret" },
    })).rejects.toMatchObject({ code: "alias_already_declared", status: 409 });
  });
});
