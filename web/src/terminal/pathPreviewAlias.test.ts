import { describe, expect, it } from "vitest";
import { localHostAlias } from "../sftp/localHost";
import { pathPreviewAlias } from "./pathPreviewAlias";

describe("pathPreviewAlias", () => {
  it("reads a shell's files on the machine of the engine that runs it", () => {
    expect(pathPreviewAlias({ kind: "shell" })).toBe(localHostAlias);
  });

  it("reads an SSH session's files on its saved connection", () => {
    expect(pathPreviewAlias({ kind: "ssh", alias: "web" })).toBe("web");
  });

  it("reads nothing for a session opened to a destination that is not a saved connection", () => {
    expect(pathPreviewAlias({ kind: "ssh" })).toBeUndefined();
  });
});
