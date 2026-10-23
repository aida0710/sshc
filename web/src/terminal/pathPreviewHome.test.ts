import { afterEach, describe, expect, it, vi } from "vitest";
import { sftpApi } from "../sftp/api";
import { localHostAlias } from "../sftp/localHost";
import { pathPreviewAlias } from "./pathPreviewAlias";
import { pathPreviewHome } from "./pathPreviewHome";

afterEach(() => vi.restoreAllMocks());

describe("pathPreviewHome", () => {
  it("uses the engine's home for a local shell", async () => {
    const listLocal = vi.spyOn(sftpApi, "listLocal").mockResolvedValue({ home: "/users/local", path: "/users/local", entries: [] });
    expect(await pathPreviewHome(localHostAlias)).toBe("/users/local");
    expect(listLocal).toHaveBeenCalledWith("~");
  });

  it.each([
    { session: { kind: "shell" as const }, runTarget: "build-box", alias: "build-box" },
    { session: { kind: "ssh" as const, alias: "web" }, runTarget: undefined, alias: "web" },
  ])("uses SFTP's home on $alias", async ({ session, runTarget, alias }) => {
    const list = vi.spyOn(sftpApi, "list").mockResolvedValue({ path: "/srv/account", entries: [] });
    const local = vi.spyOn(sftpApi, "listLocal");
    const previewAlias = pathPreviewAlias(session, runTarget);
    expect(previewAlias).toBe(alias);
    expect(await pathPreviewHome(previewAlias!)).toBe("/srv/account");
    expect(list).toHaveBeenCalledWith(alias, "~");
    expect(local).not.toHaveBeenCalled();
  });

  it("does not substitute the local home when the remote lookup fails", async () => {
    vi.spyOn(sftpApi, "list").mockRejectedValue(new Error("unavailable"));
    const local = vi.spyOn(sftpApi, "listLocal");
    await expect(pathPreviewHome("web")).rejects.toThrow("unavailable");
    expect(local).not.toHaveBeenCalled();
  });
});
