import { describe, expect, it } from "vitest";
import { localHostAlias } from "../sftp/localHost";
import { pathPreviewAlias } from "./pathPreviewAlias";
import { savedConnectionOf } from "./terminalPresentation";

describe("pathPreviewAlias", () => {
  it("reads a path printed in a shell from this machine", () => {
    expect(pathPreviewAlias({ kind: "shell" }, undefined, false)).toBe(localHostAlias);
  });

  it("reads a path printed in a shell on a run target from that run target", () => {
    expect(pathPreviewAlias({ kind: "shell" }, "build-box", false)).toBe("build-box");
  });

  it("reads a path printed in an SSH session from its alias", () => {
    expect(pathPreviewAlias({ kind: "ssh", alias: "web" }, undefined, false)).toBe("web");
  });

  it("has no alias for an SSH session opened on a destination that was not saved", () => {
    expect(pathPreviewAlias({ kind: "ssh", alias: "deploy@10.0.0.5" }, undefined, false)).toBeUndefined();
  });

  it("has no alias for an SSH session opened on a run target whose alias reaches another host from here", () => {
    expect(pathPreviewAlias({ kind: "ssh", alias: "web" }, "build-box", false)).toBeUndefined();
  });

  it("reads a path printed in an SSH session on a run target from its alias when it reaches the same host", () => {
    expect(pathPreviewAlias({ kind: "ssh", alias: "web" }, "build-box", true)).toBe("web");
  });
});

describe("savedConnectionOf", () => {
  it("names the connection of an SSH session", () => {
    expect(savedConnectionOf({ kind: "ssh", alias: "web" })).toBe("web");
  });

  it("has none for a destination that was not saved, or for a shell", () => {
    expect(savedConnectionOf({ kind: "ssh", alias: "deploy@10.0.0.5" })).toBeUndefined();
    expect(savedConnectionOf({ kind: "shell" })).toBeUndefined();
  });
});
