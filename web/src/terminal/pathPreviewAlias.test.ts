import { describe, expect, it } from "vitest";
import { localHostAlias } from "../sftp/localHost";
import { pathPreviewAlias } from "./pathPreviewAlias";

describe("pathPreviewAlias", () => {
  it("reads a path printed in a shell from this PC", () => {
    expect(pathPreviewAlias({ kind: "shell" }, undefined)).toBe(localHostAlias);
  });

  it("reads a path printed in a shell on a run target from that run target", () => {
    expect(pathPreviewAlias({ kind: "shell" }, "build-box")).toBe("build-box");
  });

  it("reads a path printed in an SSH session from its alias", () => {
    expect(pathPreviewAlias({ kind: "ssh", alias: "web" }, undefined)).toBe("web");
  });

  it("has no alias for an SSH session opened on a run target", () => {
    expect(pathPreviewAlias({ kind: "ssh", alias: "web" }, "build-box")).toBeUndefined();
  });
});
