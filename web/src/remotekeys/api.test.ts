import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, apiClient } from "../api/client";
import { remoteKeysApi } from "./api";
import { sentJson } from "../testing/requests";

const csrfToken = "c".repeat(43);
const actionToken = "a".repeat(43);

const publicKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIPr0nHGmQb99GXmUofxJM4BXGwGzO0jGsQFBspODbkvS deploy@laptop";

const plan = {
  alias: "bastion",
  user: "deploy",
  hostname: "bastion.example.com",
  port: "22",
  valuesFrom: "engine",
  fingerprint: "SHA256:bytFrSjxj2qRszG8sHhWN+YO3b9vDSU3gQtMorwKpEs",
  keyPath: "~/.ssh/id_ed25519.pub",
  keyLine: publicKey,
  remotePath: "~/.ssh/authorized_keys",
  routine: "set -e\numask 077\n",
  supported: true,
  manual: ["Open a session to the host yourself."],
  executableDirectives: [],
  actionToken,
  actionExpiresAt: "2026-08-05T09:02:00Z",
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

describe("remoteKeysApi", () => {
  it("returns the confirmation minted with the described change", async () => {
    const fetcher = vi.fn().mockResolvedValueOnce(jsonResponse(plan));
    vi.stubGlobal("fetch", fetcher);

    const described = await remoteKeysApi.plan({
      alias: "bastion",
      keyPath: "~/.ssh/id_ed25519.pub",
      publicKey,
    });
    expect(described.remotePath).toBe("~/.ssh/authorized_keys");
    expect(described.actionToken).toBe(actionToken);

    expect(fetcher).toHaveBeenCalledTimes(1);
    const [path, init] = fetcher.mock.calls[0] as [string, RequestInit];
    expect(path).toBe("/api/v1/remote-keys/plan");
    const headers = new Headers(init.headers);
    expect(headers.get("X-SSHC-Action")).toBeNull();
    expect(headers.get("X-SSHC-CSRF")).toBe(csrfToken);
    expect(sentJson(init)).toEqual({
      alias: "bastion",
      keyPath: "~/.ssh/id_ed25519.pub",
      publicKey,
    });
  });

  it("registers with the token from the displayed plan without minting another", async () => {
    const fetcher = vi.fn().mockResolvedValueOnce(
      jsonResponse({ outcome: "added", exitCode: 0, stderr: "", truncated: false }),
    );
    vi.stubGlobal("fetch", fetcher);

    const result = await remoteKeysApi.register({
      alias: "bastion",
      keyPath: "~/.ssh/id_ed25519.pub",
      publicKey,
      acknowledgeExecutable: true,
      actionToken,
    });
    expect(result.outcome).toBe("added");

    expect(fetcher).toHaveBeenCalledTimes(1);
    const [registerPath, registerInit] = fetcher.mock.calls[0] as [string, RequestInit];
    expect(registerPath).toBe("/api/v1/remote-keys/register");
    expect(new Headers(registerInit.headers).get("X-SSHC-Action")).toBe(actionToken);
    expect(sentJson(registerInit)).toEqual({
      alias: "bastion",
      keyPath: "~/.ssh/id_ed25519.pub",
      publicKey,
      acknowledgeExecutable: true,
    });
    expect(window.localStorage.length).toBe(0);
    expect(window.sessionStorage.length).toBe(0);
  });

  it("raises the server's refusal code when the remote is unsupported", async () => {
    const fetcher = vi.fn().mockResolvedValueOnce(
      jsonResponse({ code: "unsupported_remote", message: "no POSIX shell" }, 422),
    );
    vi.stubGlobal("fetch", fetcher);

    const failure = await remoteKeysApi
      .register({ alias: "bastion", keyPath: "", publicKey, acknowledgeExecutable: false, actionToken })
      .catch((error: unknown) => error);

    expect(failure).toBeInstanceOf(ApiError);
    expect((failure as ApiError).code).toBe("unsupported_remote");
    expect((failure as ApiError).status).toBe(422);
  });
});
