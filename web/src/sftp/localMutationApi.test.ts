import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { apiClient, whenRequestFailed } from "../api/client";
import { localMutationApi } from "./localMutationApi";
import type { RemoteEntry } from "./api";

const notes: RemoteEntry = { name: "notes.txt", path: "C:/Users/engine/notes.txt", type: "file", size: 4, mode: "0600", modifiedAt: "2026-10-07T01:00:00Z", revision: "notes-revision" };

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

describe("engine-local mutation API", () => {
  beforeEach(() => apiClient.setCSRF("c".repeat(43)));
  afterEach(() => { apiClient.clear(); whenRequestFailed(null); });

  it("sends a child name to dedicated mkdir and rename routes with the listing revision", async () => {
    const fetcher = vi.spyOn(globalThis, "fetch").mockResolvedValue(jsonResponse(notes, 201));
    await localMutationApi.mkdir("C:/Users/engine", "folder");
    expect(fetcher).toHaveBeenLastCalledWith("/api/v1/sftp/local/directories", expect.objectContaining({ body: JSON.stringify({ directory: "C:/Users/engine", name: "folder" }) }));
    fetcher.mockResolvedValue(jsonResponse({ ...notes, name: "renamed.txt", path: "C:/Users/engine/renamed.txt" }));
    await localMutationApi.rename(notes, "renamed.txt");
    expect(fetcher).toHaveBeenLastCalledWith("/api/v1/sftp/local/rename", expect.objectContaining({ body: JSON.stringify({ path: notes.path, name: "renamed.txt", expectedRevision: notes.revision }) }));
  });

  it("binds a multiple-selection delete to the inspected revision and single-use token", async () => {
    const folder = { ...notes, name: "folder", path: "//server/share/folder", type: "directory" as const, revision: "folder-revision" };
    const fetcher = vi.spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(jsonResponse({ revision: "tree-revision", items: 5, actionToken: "fixture-token", actionExpiresAt: "2026-10-07T01:01:00Z" }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }));
    await localMutationApi.remove([notes, folder]);
    const entries = [notes, folder].map((entry) => ({ path: entry.path, expectedRevision: entry.revision }));
    expect(fetcher).toHaveBeenNthCalledWith(1, "/api/v1/sftp/local/delete-plan", expect.objectContaining({ body: JSON.stringify({ entries }) }));
    const [endpoint, request] = fetcher.mock.calls[1]!;
    expect(endpoint).toBe("/api/v1/sftp/local/delete");
    expect(JSON.parse(request?.body as string)).toEqual({ entries, expectedRevision: "tree-revision" });
    expect(new Headers(request?.headers).get("X-SSHC-Action")).toBe("fixture-token");
    expect(new Headers(request?.headers).get("X-SSHC-CSRF")).toBe("c".repeat(43));
  });

  it.each(["sftp_conflict", "sftp_traversal_limit", "sftp_permission_denied"])("does not attempt deletion when planning refuses with %s", async (code) => {
    const diagnostic = vi.fn(); whenRequestFailed(diagnostic);
    const fetcher = vi.spyOn(globalThis, "fetch").mockResolvedValue(jsonResponse({ code, message: "request rejected" }, code === "sftp_permission_denied" ? 403 : code === "sftp_traversal_limit" ? 413 : 409));
    await expect(localMutationApi.remove([notes])).rejects.toMatchObject({ code });
    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(diagnostic).not.toHaveBeenCalled();
  });

  it("rejects an invalid confirmation response before deleting", async () => {
    const fetcher = vi.spyOn(globalThis, "fetch").mockResolvedValue(jsonResponse({ revision: "tree-revision", items: 1, actionToken: "", actionExpiresAt: "2026-10-07T01:01:00Z" }));
    await expect(localMutationApi.remove([notes])).rejects.toThrow();
    expect(fetcher).toHaveBeenCalledTimes(1);
  });
});
