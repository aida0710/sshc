import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { apiClient } from "./client";
import { settingsApi } from "./settings";

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

describe("settingsApi terminal settings", () => {
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

    await expect(settingsApi.terminalSettings()).resolves.toEqual({
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

    await expect(settingsApi.terminalSettings()).resolves.toEqual({
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

    await expect(settingsApi.localShellProfiles?.()).resolves.toEqual({
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

describe("settingsApi engine settings", () => {
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

    await expect(settingsApi.engineSettings()).resolves.toEqual({
      port: 43123,
      vaultAutoLock: { mode: "idle", value: 45, unit: "minutes" },
    });
    await expect(settingsApi.engineSettings()).resolves.toEqual({
      vaultAutoLock: { mode: "restart" },
    });
    await expect(settingsApi.engineSettings()).resolves.toEqual({});
  });
});
