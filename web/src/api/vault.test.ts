import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { apiClient } from "./client";
import { vaultApi } from "./vault";
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

describe("vaultApi vault format recovery", () => {
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
      vaultApi.recoverCompatibleVault("master password"),
    ).resolves.toEqual(status);

    const [path, init] = fetcher.mock.calls[0] as [string, RequestInit];
    expect(path).toBe("/api/v1/passwords/recover-compatible-backup");
    expect(sentJson(init)).toEqual({ passphrase: "master password" });
  });

  it("sends the explicit destructive acknowledgement when resetting", async () => {
    const fetcher = vi.fn().mockResolvedValue(jsonResponse(status));
    vi.stubGlobal("fetch", fetcher);

    await expect(
      vaultApi.resetUnsupportedVault("master password"),
    ).resolves.toEqual(status);

    const [path, init] = fetcher.mock.calls[0] as [string, RequestInit];
    expect(path).toBe("/api/v1/passwords/reset-unsupported");
    expect(sentJson(init)).toEqual({
      passphrase: "master password",
      acknowledged: true,
    });
  });
});

describe("vaultApi.passwordVault", () => {
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

    await expect(vaultApi.passwordVault()).resolves.toMatchObject({
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

    await expect(vaultApi.passwordVault()).resolves.toMatchObject({
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

    await expect(vaultApi.passwordVault()).rejects.toThrow(
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

    await expect(vaultApi.passwordVault()).rejects.toThrow(
      "invalid_response",
    );
  });
});
