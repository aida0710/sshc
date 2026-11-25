import { afterEach, describe, expect, it, vi } from "vitest";
import { sftpApi } from "../sftp/api";
import { localHostAlias } from "../sftp/localHost";
import { pathPreviewAlias } from "./pathPreviewAlias";
import { pathPreviewHome } from "./pathPreviewHome";

afterEach(() => vi.restoreAllMocks());

describe("pathPreviewHome", () => {
  it("uses the engine's home for a local shell", async () => {
    const listLocal = vi.spyOn(sftpApi, "listLocal").mockResolvedValue({ home: "/users/local", path: "/users/local", entries: [] });
    expect(await pathPreviewHome(sftpApi, localHostAlias)).toBe("/users/local");
    expect(listLocal).toHaveBeenCalledWith("~");
  });

  it("uses the engine's home for a shell and SFTP's home for an SSH session, whatever engine runs them", async () => {
    expect(pathPreviewAlias({ kind: "shell" })).toBe(localHostAlias);
    const list = vi.spyOn(sftpApi, "list").mockResolvedValue({ path: "/srv/account", entries: [] });
    const local = vi.spyOn(sftpApi, "listLocal");
    const previewAlias = pathPreviewAlias({ kind: "ssh", alias: "web" });
    expect(previewAlias).toBe("web");
    expect(await pathPreviewHome(sftpApi, previewAlias!)).toBe("/srv/account");
    expect(list).toHaveBeenCalledWith("web", "~");
    expect(local).not.toHaveBeenCalled();
  });

  it("does not substitute the local home when the remote lookup fails", async () => {
    vi.spyOn(sftpApi, "list").mockRejectedValue(new Error("unavailable"));
    const local = vi.spyOn(sftpApi, "listLocal");
    await expect(pathPreviewHome(sftpApi, "web")).rejects.toThrow("unavailable");
    expect(local).not.toHaveBeenCalled();
  });
});
