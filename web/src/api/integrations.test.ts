import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, apiClient } from "./client";
import { integrationsApi, KNOWN_HOSTS_ADD_ACTION_KIND } from "./integrations";
import { sentJson } from "../testing/requests";

const csrfToken = "c".repeat(43);
const actionToken = "a".repeat(43);

const candidate = {
  host: "new.example.com",
  port: 22,
  keyType: "ssh-ed25519",
  key: "AAAAC3NzaC1lZDI1NTE5AAAAIPr0nHGmQb99GXmUofxJM4BXGwGzO0jGsQFBspODbkvS",
};

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

describe("integrationsApi.addKnownHost", () => {
  it("uses the committed action vocabulary", () => {
    expect(KNOWN_HOSTS_ADD_ACTION_KIND).toBe("known_hosts.add");
  });

  it("mints a token bound to the host and sends no evidence of its own", async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ token: actionToken, expiresAt: "2026-08-05T09:02:00Z" }, 201))
      .mockResolvedValueOnce(jsonResponse({ changed: true, transactionId: "tx-2" }));
    vi.stubGlobal("fetch", fetcher);

    const result = await integrationsApi.addKnownHost(candidate, "SHA256:proof", false);
    expect(result.transactionId).toBe("tx-2");

    const [actionPath, actionInit] = fetcher.mock.calls[0] as [string, RequestInit];
    expect(actionPath).toBe("/api/v1/actions");
    expect(sentJson(actionInit)).toEqual({
      kind: "known_hosts.add",
      target: "new.example.com",
    });

    const [addPath, addInit] = fetcher.mock.calls[1] as [string, RequestInit];
    expect(addPath).toBe("/api/v1/known-hosts/add");
    const headers = new Headers(addInit.headers);
    expect(headers.get("X-SSHC-Action")).toBe(actionToken);
    expect(headers.get("X-SSHC-CSRF")).toBe(csrfToken);
    expect(sentJson(addInit)).toEqual({
      ...candidate,
      expectedFingerprint: "SHA256:proof",
      acknowledged: false,
    });
    expect(window.localStorage.length).toBe(0);
    expect(window.sessionStorage.length).toBe(0);
  });

  it("raises the server's refusal code when the add is rejected", async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ token: actionToken, expiresAt: "2026-08-05T09:02:00Z" }, 201))
      .mockResolvedValueOnce(
        jsonResponse({ code: "unverified_candidate", message: "not proven" }, 409),
      );
    vi.stubGlobal("fetch", fetcher);

    const failure = await integrationsApi.addKnownHost(candidate, "", false).catch((error: unknown) => error);
    expect(failure).toBeInstanceOf(ApiError);
    expect((failure as ApiError).code).toBe("unverified_candidate");
    expect((failure as ApiError).status).toBe(409);
  });
});

describe("integrationsApi terminal sessions", () => {
  const session = {
    id: "3f9c", kind: "shell", title: "zsh", startedAt: "2026-08-13T09:00:00Z",
  };

  it("opens a session and returns the single-use stream ticket", async () => {
    const fetcher = vi.fn().mockResolvedValue(jsonResponse({ session, streamTicket: "one-time" }));
    vi.stubGlobal("fetch", fetcher);

    await expect(integrationsApi.openTerminalSession({ kind: "shell" }))
      .resolves.toEqual({ session, streamTicket: "one-time" });

    const [path, init] = fetcher.mock.calls[0] as [string, RequestInit];
    expect(path).toBe("/api/v1/terminal/sessions");
    expect(init.method).toBe("POST");
    expect(new Headers(init.headers).get("X-SSHC-CSRF")).toBe(csrfToken);
    expect(new Headers(init.headers).get("X-SSHC-Action")).toBeNull();
  });

  it.each([
    { sessions: [{ ...session, kind: "telnet" }], maxSessions: 50 },
    { sessions: [{ ...session, id: 3 }], maxSessions: 50 },
    { sessions: [{ ...session, exited: { code: "0", signal: "", at: "" } }], maxSessions: 50 },
    { sessions: [], maxSessions: -1 },
    { sessions: {}, maxSessions: 50 },
  ])("rejects a malformed session list %#", async (body) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(body)));

    await expect(integrationsApi.terminalSessions()).rejects.toThrow("invalid_response");
  });
});

describe("integrationsApi terminal settings", () => {
  it("keeps explicitly disabled clipboard choices and leaves absent defaults unset", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({
      schemaVersion: 3,
      embeddedTerminal: { copyOnSelect: false, rightClickPaste: false },
    })));

    await expect(integrationsApi.terminalSettings()).resolves.toEqual({
      copyOnSelect: false,
      rightClickPaste: false,
    });
  });

  it("brings every stored field back", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({
      schemaVersion: 3,
      embeddedTerminal: {
        startDirectory: "~/work",
        maxSessions: 4,
        scrollbackBytes: 65536,
        fontSize: 18,
        copyOnSelect: false,
        rightClickPaste: false,
      },
    })));

    await expect(integrationsApi.terminalSettings()).resolves.toEqual({
      startDirectory: "~/work",
      maxSessions: 4,
      scrollbackBytes: 65536,
      fontSize: 18,
      copyOnSelect: false,
      rightClickPaste: false,
    });
  });
});

describe("integrationsApi.passwordVault", () => {
  it("accepts dedicated key-passphrase subjects", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({
      exists: true,
      unlocked: true,
      aliases: ["edge"],
      dedicatedKeyPassphrases: ["keys/id_edge"],
      minPassphraseLength: 12,
    })));

    await expect(integrationsApi.passwordVault()).resolves.toMatchObject({
      dedicatedKeyPassphrases: ["keys/id_edge"],
    });
  });

  it.each([
    { exists: true, unlocked: true, aliases: [] },
    { exists: true, unlocked: true, aliases: [], dedicatedKeyPassphrases: "keys/id_edge" },
    { exists: true, unlocked: true, aliases: [], dedicatedKeyPassphrases: [false] },
  ])("rejects a malformed dedicated key-passphrase status", async (body) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(body)));

    await expect(integrationsApi.passwordVault()).rejects.toThrow("invalid_response");
  });
});

describe("integrationsApi.credentials", () => {
  it("accepts named and dedicated host assignments", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({
      credentials: [
        { kind: "password", name: "office", uses: ["web-1"], hosts: ["web-1"] },
        { kind: "key_passphrase", name: "team", uses: ["keys/id_team"], hosts: ["build"] },
      ],
      dedicatedKeyPassphrases: [{ key: "keys/id_owned", hosts: ["deploy"] }],
      keyHostUsageComplete: true,
    })));

    await expect(integrationsApi.credentials()).resolves.toMatchObject({
      dedicatedKeyPassphrases: [{ key: "keys/id_owned", hosts: ["deploy"] }],
      keyHostUsageComplete: true,
    });
  });

  it.each([
    {
      credentials: [{ kind: "password", name: "office", uses: [] }],
      dedicatedKeyPassphrases: [], keyHostUsageComplete: true,
    },
    { credentials: [], dedicatedKeyPassphrases: "keys/id_owned", keyHostUsageComplete: true },
    { credentials: [], dedicatedKeyPassphrases: [{ key: false, hosts: [] }], keyHostUsageComplete: true },
    { credentials: [], dedicatedKeyPassphrases: [{ key: "keys/id_owned", hosts: [false] }], keyHostUsageComplete: true },
    { credentials: [], dedicatedKeyPassphrases: [], keyHostUsageComplete: "yes" },
  ])("rejects malformed credential usage", async (body) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(body)));

    await expect(integrationsApi.credentials()).rejects.toThrow("invalid_response");
  });
});

describe("integrationsApi remote sync measurements", () => {
  const summary = {
    createdAt: "2026-08-12T01:02:03Z",
    fileCount: 3,
    sourceBytes: 1200,
    snapshotBytes: 900,
  };
  const status = {
    configured: true,
    keyConfigured: true,
    locked: false,
    auto: { enabled: false, phase: "idle" as const },
    endpoint: "https://s3.example.invalid",
    bucket: "sshc",
    synced: true,
    direction: "both" as const,
    lastOperation: {
      kind: "push" as const,
      summary,
      objectCount: 2,
      uploadedBytes: 1800,
      completedAt: "2026-08-12T01:02:04Z",
    },
  };

  it("returns the push result separately from the refreshed status", async () => {
    const response = {
      status,
      result: {
        summary,
        objectCount: 2,
        uploadedBytes: 1800,
        completedAt: "2026-08-12T01:02:04Z",
      },
    };
    const fetcher = vi.fn().mockResolvedValue(jsonResponse(response));
    vi.stubGlobal("fetch", fetcher);

    await expect(integrationsApi.pushSnapshot()).resolves.toEqual(response);
    const [path, init] = fetcher.mock.calls[0] as [string, RequestInit];
    expect(path).toBe("/api/v1/sync/push");
    expect(sentJson(init)).toEqual({});
  });

  it("accepts the measured result of an apply download", async () => {
    const response = {
      applied: true,
      conflicts: [],
      written: ["config"],
      removed: [],
      summary,
      downloadedBytes: 900,
      completedAt: "2026-08-12T01:03:00Z",
    };
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(response)));

    await expect(integrationsApi.pullSnapshot(true)).resolves.toEqual(response);
  });

  it.each([
    { status, result: { summary, objectCount: 2, uploadedBytes: -1, completedAt: "now" } },
    { status, result: { summary: { ...summary, fileCount: "three" }, objectCount: 2, uploadedBytes: 1800, completedAt: "now" } },
    { status: { ...status, lastOperation: { ...status.lastOperation, kind: "copy" } }, result: { summary, objectCount: 2, uploadedBytes: 1800, completedAt: "now" } },
  ])("rejects malformed push measurements %#", async (body) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(body)));

    await expect(integrationsApi.pushSnapshot()).rejects.toThrow("invalid_response");
  });

  it.each([
    { applied: false, conflicts: [], written: [], removed: [], downloadedBytes: 900, completedAt: "now" },
    { applied: false, conflicts: [], written: [], removed: [], summary, downloadedBytes: "900", completedAt: "now" },
    { applied: false, conflicts: [], written: [], removed: [], summary, downloadedBytes: 900 },
  ])("rejects malformed pull measurements %#", async (body) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(body)));

    await expect(integrationsApi.pullSnapshot(false)).rejects.toThrow("invalid_response");
  });
});

describe("integrationsApi.syncStatus", () => {
  const cleanInstall = {
    configured: false,
    keyConfigured: false,
    locked: true,
    auto: { enabled: false, phase: "idle" },
    endpoint: "",
    bucket: "",
    path: "",
    region: "",
    synced: false,
    direction: "both",
  };

  it("accepts the complete unconfigured status returned by a fresh engine", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(cleanInstall)));

    await expect(integrationsApi.syncStatus()).resolves.toEqual(cleanInstall);
  });

  it.each([
    { ...cleanInstall, auto: { enabled: false, phase: "" } },
    { ...cleanInstall, auto: { enabled: false } },
  ])("rejects a status outside the auto phase contract %#", async (body) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(body)));

    await expect(integrationsApi.syncStatus()).rejects.toThrow();
  });
});
