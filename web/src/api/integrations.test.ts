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
      .mockResolvedValueOnce(
        jsonResponse(
          { token: actionToken, expiresAt: "2026-08-05T09:02:00Z" },
          201,
        ),
      )
      .mockResolvedValueOnce(
        jsonResponse({ changed: true, transactionId: "tx-2" }),
      );
    vi.stubGlobal("fetch", fetcher);

    const result = await integrationsApi.addKnownHost(
      candidate,
      "SHA256:proof",
      false,
    );
    expect(result.transactionId).toBe("tx-2");

    const [actionPath, actionInit] = fetcher.mock.calls[0] as [
      string,
      RequestInit,
    ];
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
      .mockResolvedValueOnce(
        jsonResponse(
          { token: actionToken, expiresAt: "2026-08-05T09:02:00Z" },
          201,
        ),
      )
      .mockResolvedValueOnce(
        jsonResponse(
          { code: "unverified_candidate", message: "not proven" },
          409,
        ),
      );
    vi.stubGlobal("fetch", fetcher);

    const failure = await integrationsApi
      .addKnownHost(candidate, "", false)
      .catch((error: unknown) => error);
    expect(failure).toBeInstanceOf(ApiError);
    expect((failure as ApiError).code).toBe("unverified_candidate");
    expect((failure as ApiError).status).toBe(409);
  });
});

describe("integrationsApi terminal sessions", () => {
  const session = {
    id: "3f9c",
    kind: "shell",
    title: "zsh",
    startedAt: "2026-08-13T09:00:00Z",
    state: "connected",
    problem: "",
  };

  it("opens a session and returns the single-use stream ticket", async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValue(jsonResponse({ session, streamTicket: "one-time" }));
    vi.stubGlobal("fetch", fetcher);

    await expect(
      integrationsApi.openTerminalSession({ kind: "shell" }),
    ).resolves.toEqual({ session, streamTicket: "one-time" });

    const [path, init] = fetcher.mock.calls[0] as [string, RequestInit];
    expect(path).toBe("/api/v1/terminal/sessions");
    expect(init.method).toBe("POST");
    expect(new Headers(init.headers).get("X-SSHC-CSRF")).toBe(csrfToken);
    expect(new Headers(init.headers).get("X-SSHC-Action")).toBeNull();
  });

  it("reconnects an exited session without an action token", async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValue(
        jsonResponse({ sessions: [session], maxSessions: 50 }),
      );
    vi.stubGlobal("fetch", fetcher);

    await expect(
      integrationsApi.reconnectTerminalSession("session id"),
    ).resolves.toEqual({ sessions: [session], maxSessions: 50 });

    const [path, init] = fetcher.mock.calls[0] as [string, RequestInit];
    expect(path).toBe("/api/v1/terminal/sessions/session%20id/reconnect");
    expect(init.method).toBe("POST");
    expect(new Headers(init.headers).get("X-SSHC-CSRF")).toBe(csrfToken);
    expect(new Headers(init.headers).get("X-SSHC-Action")).toBeNull();
  });

  it("starts and stops a temporary forward on one encoded session", async () => {
    const forwarded = {
      ...session,
      kind: "ssh",
      forwards: [
        {
          id: "pf-1",
          kind: "dynamic",
          listen: "127.0.0.1:1080",
          to: "",
          problem: "",
          temporary: true,
        },
      ],
    };
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce(
        jsonResponse({ sessions: [forwarded], maxSessions: 50 }, 201),
      )
      .mockResolvedValueOnce(
        jsonResponse({
          sessions: [{ ...forwarded, forwards: [] }],
          maxSessions: 50,
        }),
      );
    vi.stubGlobal("fetch", fetcher);

    await integrationsApi.startTerminalForward("session id", {
      kind: "dynamic",
      listenPort: 1080,
    });
    await integrationsApi.stopTerminalForward("session id", "pf/1");

    expect(fetcher.mock.calls[0]?.[0]).toBe(
      "/api/v1/terminal/sessions/session%20id/forwards",
    );
    expect(sentJson(fetcher.mock.calls[0]?.[1] as RequestInit)).toEqual({
      kind: "dynamic",
      listenPort: 1080,
    });
    expect(fetcher.mock.calls[1]?.[0]).toBe(
      "/api/v1/terminal/sessions/session%20id/forwards/pf%2F1",
    );
    expect((fetcher.mock.calls[1]?.[1] as RequestInit).method).toBe("DELETE");
  });

  it.each([
    { sessions: [{ ...session, kind: "telnet" }], maxSessions: 50 },
    { sessions: [{ ...session, id: 3 }], maxSessions: 50 },
    { sessions: [{ ...session, state: "lost" }], maxSessions: 50 },
    { sessions: [{ ...session, problem: 3 }], maxSessions: 50 },
    {
      sessions: [
        {
          ...session,
          progress: {
            phase: "waiting",
            alias: "edge",
            hostName: "edge",
            user: "ops",
            hop: 1,
            hops: 2,
          },
        },
      ],
      maxSessions: 50,
    },
    {
      sessions: [
        {
          ...session,
          progress: {
            phase: "authenticating",
            alias: "edge",
            hostName: "edge",
            user: "ops",
            hop: 3,
            hops: 2,
          },
        },
      ],
      maxSessions: 50,
    },
    {
      sessions: [
        {
          ...session,
          state: "reconnecting",
          reconnect: { attempt: 0, limit: 5, retryAt: "x", problem: "" },
        },
      ],
      maxSessions: 50,
    },
    {
      sessions: [{ ...session, exited: { code: "0", signal: "", at: "" } }],
      maxSessions: 50,
    },
    {
      sessions: [
        {
          ...session,
          forwards: [
            {
              id: "",
              kind: "remote",
              listen: "x",
              to: "y",
              problem: "",
              temporary: false,
            },
          ],
        },
      ],
      maxSessions: 50,
    },
    {
      sessions: [
        {
          ...session,
          forwards: [{ kind: "local", listen: "x", to: "y", problem: "" }],
        },
      ],
      maxSessions: 50,
    },
    { sessions: [], maxSessions: -1 },
    { sessions: {}, maxSessions: 50 },
  ])("rejects a malformed session list %#", async (body) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(body)));

    await expect(integrationsApi.terminalSessions()).rejects.toThrow(
      "invalid_response",
    );
  });
});

describe("integrationsApi recent connections", () => {
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

    await expect(integrationsApi.recentConnections()).resolves.toEqual({
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
    await expect(integrationsApi.recentConnections()).rejects.toThrow(
      "invalid_response",
    );
  });
});

describe("integrationsApi vault format recovery", () => {
  const status = {
    exists: true,
    unlocked: true,
    aliases: [],
    dedicatedKeyPassphrases: [],
  };

  it("requests the newest compatible backup with the supplied master password", async () => {
    const fetcher = vi.fn().mockResolvedValue(jsonResponse(status));
    vi.stubGlobal("fetch", fetcher);

    await expect(
      integrationsApi.recoverCompatibleVault("master password"),
    ).resolves.toEqual(status);

    const [path, init] = fetcher.mock.calls[0] as [string, RequestInit];
    expect(path).toBe("/api/v1/passwords/recover-compatible-backup");
    expect(sentJson(init)).toEqual({ passphrase: "master password" });
  });

  it("sends the explicit destructive acknowledgement when resetting", async () => {
    const fetcher = vi.fn().mockResolvedValue(jsonResponse(status));
    vi.stubGlobal("fetch", fetcher);

    await expect(
      integrationsApi.resetUnsupportedVault("master password"),
    ).resolves.toEqual(status);

    const [path, init] = fetcher.mock.calls[0] as [string, RequestInit];
    expect(path).toBe("/api/v1/passwords/reset-unsupported");
    expect(sentJson(init)).toEqual({
      passphrase: "master password",
      acknowledged: true,
    });
  });
});

describe("integrationsApi terminal settings", () => {
  it("keeps explicitly disabled clipboard choices and leaves absent defaults unset", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        jsonResponse({
          schemaVersion: 3,
          embeddedTerminal: { copyOnSelect: false, rightClickPaste: false },
        }),
      ),
    );

    await expect(integrationsApi.terminalSettings()).resolves.toEqual({
      copyOnSelect: false,
      rightClickPaste: false,
    });
  });

  it("brings every stored field back", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        jsonResponse({
          schemaVersion: 3,
          embeddedTerminal: {
            startDirectory: "~/work",
            maxSessions: 4,
            scrollbackBytes: 65536,
            browserScrollbackLines: 12000,
            fontSize: 18,
            copyOnSelect: false,
            rightClickPaste: false,
            osc52: true,
            jisYenBackslash: true,
            localShellProfile: "fish",
          },
        }),
      ),
    );

    await expect(integrationsApi.terminalSettings()).resolves.toEqual({
      startDirectory: "~/work",
      maxSessions: 4,
      scrollbackBytes: 65536,
      browserScrollbackLines: 12000,
      fontSize: 18,
      copyOnSelect: false,
      rightClickPaste: false,
      osc52: true,
      jisYenBackslash: true,
      localShellProfile: "fish",
    });
  });

  it("validates detected local shell profiles", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        jsonResponse({
          profiles: [
            {
              id: "fish",
              label: "fish",
              path: "/usr/bin/fish",
              arguments: [],
              default: true,
            },
          ],
        }),
      ),
    );

    await expect(integrationsApi.localShellProfiles?.()).resolves.toEqual({
      profiles: [
        {
          id: "fish",
          label: "fish",
          path: "/usr/bin/fish",
          arguments: [],
          default: true,
        },
      ],
    });
  });
});

describe("integrationsApi engine settings", () => {
  it("restores timed and restart-only Vault locking without inventing defaults", async () => {
    const fetcher = vi.fn()
      .mockResolvedValueOnce(jsonResponse({
        schemaVersion: 4,
        engine: { port: 43123, vaultAutoLock: { mode: "idle", value: 45, unit: "minutes" } },
      }))
      .mockResolvedValueOnce(jsonResponse({
        schemaVersion: 4,
        engine: { vaultAutoLock: { mode: "restart" } },
      }))
      .mockResolvedValueOnce(jsonResponse({ schemaVersion: 4 }));
    vi.stubGlobal("fetch", fetcher);

    await expect(integrationsApi.engineSettings()).resolves.toEqual({
      port: 43123,
      vaultAutoLock: { mode: "idle", value: 45, unit: "minutes" },
    });
    await expect(integrationsApi.engineSettings()).resolves.toEqual({
      vaultAutoLock: { mode: "restart" },
    });
    await expect(integrationsApi.engineSettings()).resolves.toEqual({});
  });
});

describe("integrationsApi.passwordVault", () => {
  it("accepts dedicated key-passphrase subjects", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        jsonResponse({
          exists: true,
          unlocked: true,
          aliases: ["edge"],
          dedicatedKeyPassphrases: ["keys/id_edge"],
          minPassphraseLength: 12,
        }),
      ),
    );

    await expect(integrationsApi.passwordVault()).resolves.toMatchObject({
      dedicatedKeyPassphrases: ["keys/id_edge"],
    });
  });

  it("accepts the safe version pair reported after a vault migration", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        jsonResponse({
          exists: true,
          unlocked: true,
          aliases: [],
          dedicatedKeyPassphrases: [],
          migratedFromVersion: 4,
          migratedToVersion: 5,
        }),
      ),
    );

    await expect(integrationsApi.passwordVault()).resolves.toMatchObject({
      migratedFromVersion: 4,
      migratedToVersion: 5,
    });
  });

  it.each([
    { migratedFromVersion: "4", migratedToVersion: 5 },
    { migratedFromVersion: 4, migratedToVersion: "5" },
  ])("rejects a malformed migration status", async (migration) => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        jsonResponse({
          exists: true,
          unlocked: true,
          aliases: [],
          dedicatedKeyPassphrases: [],
          ...migration,
        }),
      ),
    );

    await expect(integrationsApi.passwordVault()).rejects.toThrow(
      "invalid_response",
    );
  });

  it.each([
    { exists: true, unlocked: true, aliases: [] },
    {
      exists: true,
      unlocked: true,
      aliases: [],
      dedicatedKeyPassphrases: "keys/id_edge",
    },
    {
      exists: true,
      unlocked: true,
      aliases: [],
      dedicatedKeyPassphrases: [false],
    },
  ])("rejects a malformed dedicated key-passphrase status", async (body) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(body)));

    await expect(integrationsApi.passwordVault()).rejects.toThrow(
      "invalid_response",
    );
  });
});

describe("integrationsApi.credentials", () => {
  it("accepts named and dedicated host assignments", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        jsonResponse({
          credentials: [
            {
              kind: "password",
              name: "office",
              uses: ["web-1"],
              hosts: ["web-1"],
            },
            {
              kind: "key_passphrase",
              name: "team",
              uses: ["keys/id_team"],
              hosts: ["build"],
            },
          ],
          dedicatedKeyPassphrases: [
            { key: "keys/id_owned", hosts: ["deploy"] },
          ],
          keyHostUsageComplete: true,
        }),
      ),
    );

    await expect(integrationsApi.credentials()).resolves.toMatchObject({
      dedicatedKeyPassphrases: [{ key: "keys/id_owned", hosts: ["deploy"] }],
      keyHostUsageComplete: true,
    });
  });

  it.each([
    {
      credentials: [{ kind: "password", name: "office", uses: [] }],
      dedicatedKeyPassphrases: [],
      keyHostUsageComplete: true,
    },
    {
      credentials: [],
      dedicatedKeyPassphrases: "keys/id_owned",
      keyHostUsageComplete: true,
    },
    {
      credentials: [],
      dedicatedKeyPassphrases: [{ key: false, hosts: [] }],
      keyHostUsageComplete: true,
    },
    {
      credentials: [],
      dedicatedKeyPassphrases: [{ key: "keys/id_owned", hosts: [false] }],
      keyHostUsageComplete: true,
    },
    {
      credentials: [],
      dedicatedKeyPassphrases: [],
      keyHostUsageComplete: "yes",
    },
  ])("rejects malformed credential usage", async (body) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(body)));

    await expect(integrationsApi.credentials()).rejects.toThrow(
      "invalid_response",
    );
  });

  it("uses a one-time token to reveal one explicitly edited credential", async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce(
        jsonResponse(
          { token: actionToken, expiresAt: "2026-08-05T09:02:00Z" },
          201,
        ),
      )
      .mockResolvedValueOnce(
        jsonResponse({
          kind: "password",
          name: "office vm",
          secret: "saved-value",
        }),
      );
    vi.stubGlobal("fetch", fetcher);

    await expect(
      integrationsApi.revealCredential("password", "office vm"),
    ).resolves.toEqual({
      kind: "password",
      name: "office vm",
      secret: "saved-value",
    });
    const [, actionInit] = fetcher.mock.calls[0] as [string, RequestInit];
    expect(sentJson(actionInit)).toEqual({
      kind: "credential.reveal",
      target: "password\noffice vm",
    });
    const [path, init] = fetcher.mock.calls[1] as [string, RequestInit];
    expect(path).toBe("/api/v1/credentials/password/office%20vm/reveal");
    expect(new Headers(init.headers).get("X-SSHC-Action")).toBe(actionToken);
  });

  it("patches the old name with the edited name and value", async () => {
    const response = {
      credentials: [],
      dedicatedKeyPassphrases: [],
      keyHostUsageComplete: true,
    };
    const fetcher = vi.fn().mockResolvedValue(jsonResponse(response));
    vi.stubGlobal("fetch", fetcher);

    await expect(
      integrationsApi.updateCredential(
        "key_passphrase",
        "old name",
        "new name",
        "new phrase",
      ),
    ).resolves.toEqual(response);
    const [path, init] = fetcher.mock.calls[0] as [string, RequestInit];
    expect(path).toBe("/api/v1/credentials/key_passphrase/old%20name");
    expect(init.method).toBe("PATCH");
    expect(sentJson(init)).toEqual({ name: "new name", secret: "new phrase" });
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

    await expect(
      integrationsApi.pushSnapshot("Update config"),
    ).resolves.toEqual(response);
    const [path, init] = fetcher.mock.calls[0] as [string, RequestInit];
    expect(path).toBe("/api/v1/sync/push");
    expect(sentJson(init)).toEqual({ message: "Update config" });
  });

  it("binds a force push to a one-time confirmation token", async () => {
    const response = {
      status,
      result: {
        summary,
        objectCount: 2,
        uploadedBytes: 1800,
        completedAt: "2026-08-12T01:02:04Z",
      },
    };
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce(
        jsonResponse({
          token: "action-token",
          expiresAt: "2026-08-12T01:04:04Z",
        }),
      )
      .mockResolvedValueOnce(jsonResponse(response));
    vi.stubGlobal("fetch", fetcher);

    await expect(
      integrationsApi.forcePushSnapshot("Replace remote workspace"),
    ).resolves.toEqual(response);
    expect(fetcher).toHaveBeenCalledTimes(2);
    const firstCall = fetcher.mock.calls[0] as
      [string, RequestInit] | undefined;
    expect(firstCall).toBeDefined();
    expect(sentJson(firstCall![1])).toEqual({
      kind: "sync.force_push",
      target: "remote-workspace",
    });
    const [path, init] = fetcher.mock.calls[1] as [string, RequestInit];
    expect(path).toBe("/api/v1/sync/force-push");
    expect(new Headers(init.headers).get("X-SSHC-Action")).toBe("action-token");
    expect(sentJson(init)).toEqual({ message: "Replace remote workspace" });
  });

  it("validates live and history metadata read from the bucket", async () => {
    const response = {
      checkedAt: "2026-08-25T01:55:00Z",
      localIsLive: false,
      historyTruncated: false,
      live: {
        key: "workspace.tar.gz.enc",
        size: 900,
        lastModified: "2026-08-25T01:54:00Z",
      },
      history: [{ key: "snapshots/one.tar.gz.enc", size: 901 }],
    };
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(response)));

    await expect(integrationsApi.syncBucketStatus()).resolves.toEqual(response);
  });

  it("loads and saves the shared synchronization exclusions", async () => {
    const response = {
      document: "*.tmp\n",
      usingDefaults: false,
      candidates: [
        { path: "config", ignored: false },
        { path: "cache/session.tmp", ignored: true },
      ],
    };
    const fetcher = vi.fn().mockImplementation(() =>
      Promise.resolve(jsonResponse(response)),
    );
    vi.stubGlobal("fetch", fetcher);

    await expect(integrationsApi.syncExclusions()).resolves.toEqual(response);
    expect(fetcher.mock.calls[0]?.[0]).toBe("/api/v1/sync/exclusions");
    await expect(
      integrationsApi.saveSyncExclusions("*.tmp\n"),
    ).resolves.toEqual(response);
    const [path, init] = fetcher.mock.calls[1] as [string, RequestInit];
    expect(path).toBe("/api/v1/sync/exclusions");
    expect(init.method).toBe("PUT");
    expect(sentJson(init)).toEqual({ document: "*.tmp\n" });
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
      remoteETag: '"generation-1"',
      remoteRevision: "a".repeat(64),
    };
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(response)));

    await expect(
      integrationsApi.pullSnapshot(true, undefined, undefined, response),
    ).resolves.toEqual(response);
  });

  it("accepts synchronized permission details and rejects other modes", async () => {
    const response = {
      applied: false,
      conflicts: [
        {
          path: "config",
          changedHere: true,
          changedThere: true,
          baseMode: "0600",
          localMode: "0700",
          remoteMode: "0600",
        },
      ],
      written: [],
      removed: [],
      summary,
      downloadedBytes: 900,
      completedAt: "2026-08-12T01:03:00Z",
      remoteETag: '"generation-1"',
      remoteRevision: "a".repeat(64),
    };
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(response)));
    await expect(integrationsApi.pullSnapshot(false)).resolves.toEqual(response);

    const invalid = {
      ...response,
      conflicts: [{ ...response.conflicts[0], localMode: "0644" }],
    };
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(invalid)));
    await expect(integrationsApi.pullSnapshot(false)).rejects.toThrow(
      "invalid_response",
    );
  });

  it("marks an explicit receive-only remote-head preview and apply", async () => {
    const response = {
      applied: false,
      conflicts: [],
      written: ["config"],
      removed: [],
      summary,
      downloadedBytes: 900,
      completedAt: "2026-08-12T01:03:00Z",
      remoteETag: '"generation-2"',
      remoteRevision: "b".repeat(64),
    };
    const fetcher = vi.fn().mockResolvedValue(jsonResponse(response));
    vi.stubGlobal("fetch", fetcher);

    await expect(
      integrationsApi.pullSnapshot(false, "remote", undefined, undefined, true),
    ).resolves.toEqual(response);
    const [, init] = fetcher.mock.calls[0] as [string, RequestInit];
    expect(sentJson(init)).toEqual({
      apply: false,
      resolve: "remote",
      acceptRemoteHead: true,
    });
  });

  it.each([
    {
      status,
      result: {
        summary,
        objectCount: 2,
        uploadedBytes: -1,
        completedAt: "now",
      },
    },
    {
      status,
      result: {
        summary: { ...summary, fileCount: "three" },
        objectCount: 2,
        uploadedBytes: 1800,
        completedAt: "now",
      },
    },
    {
      status: {
        ...status,
        lastOperation: { ...status.lastOperation, kind: "copy" },
      },
      result: {
        summary,
        objectCount: 2,
        uploadedBytes: 1800,
        completedAt: "now",
      },
    },
  ])("rejects malformed push measurements %#", async (body) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(body)));

    await expect(integrationsApi.pushSnapshot("Update config")).rejects.toThrow(
      "invalid_response",
    );
  });

  it.each([
    {
      applied: false,
      conflicts: [],
      written: [],
      removed: [],
      downloadedBytes: 900,
      completedAt: "now",
    },
    {
      applied: false,
      conflicts: [],
      written: [],
      removed: [],
      summary,
      downloadedBytes: "900",
      completedAt: "now",
    },
    {
      applied: false,
      conflicts: [],
      written: [],
      removed: [],
      summary,
      downloadedBytes: 900,
    },
  ])("rejects malformed pull measurements %#", async (body) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(body)));

    await expect(integrationsApi.pullSnapshot(false)).rejects.toThrow(
      "invalid_response",
    );
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
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(jsonResponse(cleanInstall)),
    );

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

describe("integrationsApi.setSyncKey", () => {
  it("sends the destructive history confirmation only when the caller supplies it", async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValue(
        jsonResponse({ key: "a sufficiently long synchronization key" }),
      );
    vi.stubGlobal("fetch", fetcher);

    await integrationsApi.setSyncKey(
      "a sufficiently long synchronization key",
      true,
    );

    const [path, init] = fetcher.mock.calls[0] as [string, RequestInit];
    expect(path).toBe("/api/v1/sync/key");
    expect(sentJson(init)).toEqual({
      key: "a sufficiently long synchronization key",
      confirmHistoryLoss: true,
    });
  });
});
