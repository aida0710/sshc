import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { apiClient, whenRequestFailed } from "../api/client";
import { chmodApi } from "./chmodApi";

const selection = { entries: [{ path: "/資料:file", expectedRevision: "file-revision" }], options: { fileMode: "644", directoryMode: "755", recursive: true } };
const plan = { revision: "plan-revision", selectionCount: 1, files: 1, directories: 0, skippedSymlinks: 0, options: selection.options, actionToken: "b".repeat(43), actionExpiresAt: "2026-10-07T12:01:00Z" };

describe("permission plans", () => {
  beforeEach(() => apiClient.setCSRF("a".repeat(43)));
  afterEach(() => { apiClient.clear(); whenRequestFailed(null); vi.restoreAllMocks(); });

  it("sends the exact confirmed selection, options, revision and single-use token and reads partial outcomes", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(async (url) => new Response(JSON.stringify(
      (typeof url === "string" && url.endsWith("mode-plan")) ? plan : { applied: 1, items: 2, complete: false },
    ), { status: (typeof url === "string" && url.endsWith("mode-plan")) ? 200 : 207, headers: { "Content-Type": "application/json" } }));
    const confirmed = await chmodApi.plan("edge name", selection);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(await chmodApi.apply({ alias: "edge name", selection, plan: confirmed })).toEqual({ applied: 1, items: 2, complete: false });
    expect(fetchMock.mock.calls[1]?.[0]).toBe("/api/v1/sftp/edge%20name/modes");
    const options = fetchMock.mock.calls[1]?.[1];
    expect(JSON.parse(options?.body as string)).toEqual({ ...selection, expectedRevision: plan.revision });
    expect(new Headers(options?.headers).get("X-SSHC-Action")).toBe(plan.actionToken);
  });

  it("keeps preflight refusals local and never sends permission changes after an unsupported plan", async () => {
    const diagnostic = vi.fn();
    whenRequestFailed(diagnostic);
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({ code: "sftp_unsupported_operation", message: "request rejected" }), { status: 501, headers: { "Content-Type": "application/problem+json" } }));
    await expect(chmodApi.plan("edge", selection)).rejects.toMatchObject({ code: "sftp_unsupported_operation" });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(diagnostic).not.toHaveBeenCalled();
  });
});
