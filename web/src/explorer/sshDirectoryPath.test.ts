import { describe, expect, it } from "vitest";
import { sshDirectoryPath } from "./sshDirectoryPath";

describe("sshDirectoryPath", () => {
  it("reads a path written from ~/.ssh as the same place relative to it", () => {
    expect(sshDirectoryPath("~/.ssh/conf.d")).toEqual({ relative: "conf.d" });
    expect(sshDirectoryPath("  ~/.ssh/conf.d/30-lab.conf ")).toEqual({ relative: "conf.d/30-lab.conf" });
    expect(sshDirectoryPath("conf.d")).toEqual({ relative: "conf.d" });
  });

  it("calls a path outside ~/.ssh outside instead of nesting it", () => {
    for (const typed of ["~", "~/", "~/other", "~/.ssh/~/x", "/etc/ssh"]) {
      expect(sshDirectoryPath(typed)).toEqual({ outside: true });
    }
  });
});
