import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { apiClient } from "../../api/client";
import { sentJson } from "../../testing/requests";
import { terminalCommandApi, type TerminalCommandRequest } from "./commandApi";

const csrfToken = "c".repeat(43);
const actionToken = "a".repeat(43);

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

beforeEach(() => apiClient.setCSRF(csrfToken));

afterEach(() => {
  apiClient.clear();
  vi.unstubAllGlobals();
});

describe("terminalCommandApi", () => {
  it("previews and dispatches commands against the exact live sessions", async () => {
    const request: TerminalCommandRequest = {
      command: "pwd",
      inputs: {},
      targets: [
        { targetId: "pane-a", sessionId: "session-a" },
        { targetId: "pane-b", sessionId: "session-b" },
      ],
    };
    const prepared = {
      snippetId: "",
      evidence: "evidence",
      reviewEvidence: "review-evidence",
      actionToken,
      actionExpiresAt: "2026-08-27T10:00:00Z",
      targets: [
        { targetId: "pane-a", sessionId: "session-a", alias: "edge", title: "First", command: "pwd" },
        { targetId: "pane-b", sessionId: "session-b", alias: "edge", title: "Second", command: "pwd" },
      ],
    };
    const delivered = {
      results: prepared.targets.map((target) => ({ ...target, status: "delivered" })),
    };
    const fetcher = vi.fn()
      .mockResolvedValueOnce(jsonResponse(prepared))
      .mockResolvedValueOnce(jsonResponse(delivered));
    vi.stubGlobal("fetch", fetcher);

    const preview = await terminalCommandApi.preview(request);
    const result = await terminalCommandApi.dispatch(preview, request);

    expect(result.results.map((item) => item.sessionId)).toEqual(["session-a", "session-b"]);
    const [previewPath, previewInit] = fetcher.mock.calls[0] as [string, RequestInit];
    expect(previewPath).toBe("/api/v1/terminal/commands/preview");
    expect(sentJson(previewInit)).toEqual(request);
    expect(new Headers(previewInit.headers).get("X-SSHC-Action")).toBeNull();

    const [dispatchPath, dispatchInit] = fetcher.mock.calls[1] as [string, RequestInit];
    expect(dispatchPath).toBe("/api/v1/terminal/commands");
    expect(new Headers(dispatchInit.headers).get("X-SSHC-Action")).toBe(actionToken);
    expect(sentJson(dispatchInit)).toEqual({ ...request, submit: true, evidence: "evidence" });
  });

  it("rejects an unknown delivery status", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({
      results: [{ targetId: "pane-a", sessionId: "session-a", alias: "edge", title: "First", status: "completed" }],
    })));
    const request: TerminalCommandRequest = { command: "pwd", inputs: {}, targets: [{ targetId: "pane-a", sessionId: "session-a" }] };
    const preview = {
      snippetId: "", evidence: "evidence", reviewEvidence: "review-evidence", actionToken, actionExpiresAt: "2026-08-27T10:00:00Z", targets: [],
    };

    await expect(terminalCommandApi.dispatch(preview, request)).rejects.toThrow("invalid_response");
  });
});
