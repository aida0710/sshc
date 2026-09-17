import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { apiClient } from "./client";
import { credentialsApi } from "./credentials";
import { sentJson } from "../testing/requests";

const csrfToken = "c".repeat(43);
const actionToken = "a".repeat(43);


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

describe("credentialsApi.credentials", () => {
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

    await expect(credentialsApi.credentials()).resolves.toMatchObject({
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

    await expect(credentialsApi.credentials()).rejects.toThrow(
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
      credentialsApi.revealCredential("password", "office vm"),
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
      credentialsApi.updateCredential(
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
