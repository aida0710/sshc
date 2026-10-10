import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { apiClient, whenRequestFailed } from "../api/client";
import { remoteMetadataApi } from "./remoteMetadataApi";

const entry = { name: "link", path: "/srv/link", type: "symlink", size: 0, mode: "lrwxrwxrwx", modifiedAt: "2026-10-07T00:00:00Z", revision: "link-revision" };

describe("remote metadata API", () => {
  beforeEach(() => { apiClient.setCSRF("a".repeat(43)); });
  afterEach(() => { apiClient.clear(); whenRequestFailed(null); vi.restoreAllMocks(); });

  it("binds a Unicode and colon link target to confirmation and sends the expected revision", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(async (url) => new Response(JSON.stringify(
      url === "/api/v1/actions" ? { token: "b".repeat(43), expiresAt: "2026-10-07T00:01:00Z" } : entry,
    ), { status: url === "/api/v1/actions" ? 201 : 200, headers: { "Content-Type": "application/json" } }));
    await remoteMetadataApi.changeSymlink("edge", { path: entry.path, target: "../資料:先", expectedRevision: entry.revision });
    const confirmation = JSON.parse(fetchMock.mock.calls[0]?.[1]?.body as string) as { kind: string; target: string };
    expect(confirmation.kind).toBe("sftp.symlink");
    const encoded = confirmation.target.slice(confirmation.target.lastIndexOf(":") + 1).replaceAll("-", "+").replaceAll("_", "/");
    expect(new TextDecoder().decode(Uint8Array.from(atob(encoded), (character) => character.charCodeAt(0)))).toBe("../資料:先");
    expect(fetchMock.mock.calls[1]?.[0]).toBe("/api/v1/sftp/edge/symlink");
    expect(JSON.parse(fetchMock.mock.calls[1]?.[1]?.body as string)).toEqual({ path: entry.path, target: "../資料:先", expectedRevision: entry.revision });
    expect(new Headers(fetchMock.mock.calls[1]?.[1]?.headers).get("X-SSHC-Action")).toBe("b".repeat(43));
  });

  it("never sends a mutation after refused confirmation and keeps unsupported errors local", async () => {
    const diagnostic = vi.fn();
    whenRequestFailed(diagnostic);
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({ code: "sftp_ownership_unavailable", message: "request rejected" }), { status: 501, headers: { "Content-Type": "application/problem+json" } }));
    await expect(remoteMetadataApi.changeOwnership("edge", { path: "/srv/file", uid: 0, gid: 4294967295, expectedRevision: "rev" })).rejects.toMatchObject({ code: "sftp_ownership_unavailable" });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(diagnostic).not.toHaveBeenCalled();
  });

  it("keeps uint64 filesystem byte counts exact", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({ path: "/", availableBytes: "18446744073709551615", totalBytes: "18446744073709551615" }), { status: 200, headers: { "Content-Type": "application/json" } }));
    expect((await remoteMetadataApi.filesystemSpace("edge", "/")).availableBytes).toBe("18446744073709551615");
  });
});
