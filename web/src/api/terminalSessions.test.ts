import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { apiClient } from "./client";
import { terminalSessionsApi } from "./terminalSessions";
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

describe("terminalSessionsApi terminal sessions", () => {
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
      .mockResolvedValue(jsonResponse({ session, streamTicket: "one-time" }, 201));
    vi.stubGlobal("fetch", fetcher);

    await expect(
      terminalSessionsApi.openTerminalSession({ kind: "shell" }),
    ).resolves.toEqual({ session, streamTicket: "one-time" });

    const [path, init] = fetcher.mock.calls[0] as [string, RequestInit];
    expect(path).toBe("/api/v1/terminal/sessions");
    expect(init.method).toBe("POST");
    expect(new Headers(init.headers).get("X-SSHC-CSRF")).toBe(csrfToken);
    expect(new Headers(init.headers).get("X-SSHC-Action")).toBeNull();
  });

  it("binds a reconnect stream ticket to the browser's rendered byte cursor", async () => {
    const fetcher = vi.fn().mockResolvedValue(jsonResponse({ streamTicket: "next" }, 201));
    vi.stubGlobal("fetch", fetcher);

    await expect(terminalSessionsApi.terminalStreamTicket("session id", 8192)).resolves.toEqual({
      streamTicket: "next",
    });

    const [path, init] = fetcher.mock.calls[0] as [string, RequestInit];
    expect(path).toBe("/api/v1/terminal/sessions/session%20id/stream?cursor=8192");
    expect(init.method).toBe("POST");
  });

  it("reconnects an exited session without an action token", async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValue(
        jsonResponse({ sessions: [session], maxSessions: 50 }, 200),
      );
    vi.stubGlobal("fetch", fetcher);

    await expect(
      terminalSessionsApi.reconnectTerminalSession("session id"),
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

    await terminalSessionsApi.startTerminalForward("session id", {
      kind: "dynamic",
      listenPort: 1080,
    });
    await terminalSessionsApi.stopTerminalForward("session id", "pf/1");

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

    await expect(terminalSessionsApi.terminalSessions()).rejects.toThrow(
      "invalid_response",
    );
  });
});
