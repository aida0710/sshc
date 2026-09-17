import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { apiClient } from "./client";
import { syncApi } from "./sync";
import { sentJson } from "../testing/requests";

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

describe("syncApi remote sync measurements", () => {
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
      syncApi.pushSnapshot("Update config"),
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
          token: "a".repeat(43),
          expiresAt: "2026-08-12T01:04:04Z",
        }, 201),
      )
      .mockResolvedValueOnce(jsonResponse(response));
    vi.stubGlobal("fetch", fetcher);

    await expect(
      syncApi.forcePushSnapshot("Replace remote workspace"),
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
    expect(new Headers(init.headers).get("X-SSHC-Action")).toBe("a".repeat(43));
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

    await expect(syncApi.syncBucketStatus()).resolves.toEqual(response);
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

    await expect(syncApi.syncExclusions()).resolves.toEqual(response);
    expect(fetcher.mock.calls[0]?.[0]).toBe("/api/v1/sync/exclusions");
    await expect(
      syncApi.saveSyncExclusions("*.tmp\n"),
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
      syncApi.pullSnapshot(true, undefined, undefined, response),
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
    await expect(syncApi.pullSnapshot(false)).resolves.toEqual(response);

    const invalid = {
      ...response,
      conflicts: [{ ...response.conflicts[0], localMode: "0644" }],
    };
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(invalid)));
    await expect(syncApi.pullSnapshot(false)).rejects.toThrow(
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
      syncApi.pullSnapshot(false, "remote", undefined, undefined, true),
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

    await expect(syncApi.pushSnapshot("Update config")).rejects.toThrow(
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

    await expect(syncApi.pullSnapshot(false)).rejects.toThrow(
      "invalid_response",
    );
  });
});

describe("syncApi.syncStatus", () => {
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

    await expect(syncApi.syncStatus()).resolves.toEqual(cleanInstall);
  });

  it.each([
    { ...cleanInstall, auto: { enabled: false, phase: "" } },
    { ...cleanInstall, auto: { enabled: false } },
  ])("rejects a status outside the auto phase contract %#", async (body) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(body)));

    await expect(syncApi.syncStatus()).rejects.toThrow();
  });
});

describe("syncApi.setSyncKey", () => {
  it("sends the destructive history confirmation only when the caller supplies it", async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValue(
        jsonResponse({ key: "a sufficiently long synchronization key" }),
      );
    vi.stubGlobal("fetch", fetcher);

    await syncApi.setSyncKey(
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
