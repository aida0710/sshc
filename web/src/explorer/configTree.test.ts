import { describe, expect, it } from "vitest";
import type { FileNode } from "../api/config";
import { configTreeRows } from "./configTree";

function file(path: string, includes: { pattern: string; matches: string[] }[] = [], loads = 1): FileNode {
  return {
    file: { path, absolute: `/home/tester/.ssh/${path}` },
    editable: true,
    loads,
    includes: includes.map(({ pattern, matches }, index) => ({
      line: index + 1,
      pattern,
      matches: matches.map((match) => ({ path: match, absolute: `/home/tester/.ssh/${match}` })),
    })),
  };
}

function describeRows(files: FileNode[]): string[] {
  return configTreeRows(files).map((row) =>
    row.kind === "file" ? `${row.depth}:${row.node.file.path ?? ""}` : `${row.depth}:include ${row.include.pattern}`,
  );
}

describe("configTreeRows", () => {
  it("puts each included file under the Include that read it", () => {
    expect(describeRows([
      file("config", [{ pattern: "conf.d/*.conf", matches: ["conf.d/10-home.conf", "conf.d/20-lab.conf"] }, { pattern: "work.conf", matches: ["work.conf"] }]),
      file("conf.d/10-home.conf"),
      file("conf.d/20-lab.conf", [{ pattern: "lab/*.conf", matches: ["lab/gpu.conf"] }]),
      file("lab/gpu.conf"),
      file("work.conf"),
    ])).toEqual([
      "0:config",
      "0:include conf.d/*.conf",
      "1:conf.d/10-home.conf",
      "1:conf.d/20-lab.conf",
      "1:include lab/*.conf",
      "2:lab/gpu.conf",
      "0:include work.conf",
      "1:work.conf",
    ]);
  });

  it("shows a file read twice once, under the Include that OpenSSH reads first", () => {
    expect(describeRows([
      file("config", [{ pattern: "a.conf", matches: ["a.conf"] }, { pattern: "shared.conf", matches: ["shared.conf"] }]),
      file("a.conf", [{ pattern: "shared.conf", matches: ["shared.conf"] }]),
      file("shared.conf", [], 2),
    ])).toEqual([
      "0:config",
      "0:include a.conf",
      "1:a.conf",
      "1:include shared.conf",
      "2:shared.conf",
      "0:include shared.conf",
    ]);
  });

  it("still lists files that only include each other", () => {
    expect(describeRows([
      file("config"),
      file("a.conf", [{ pattern: "b.conf", matches: ["b.conf"] }]),
      file("b.conf", [{ pattern: "a.conf", matches: ["a.conf"] }]),
    ])).toEqual([
      "0:config",
      "0:a.conf",
      "0:include b.conf",
      "1:b.conf",
      "1:include a.conf",
    ]);
  });
});
