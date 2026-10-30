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

describe("settingsApi local shell profiles", () => {
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

