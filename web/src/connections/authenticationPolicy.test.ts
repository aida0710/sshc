import { describe, expect, it } from "vitest";
import type { HostDetail } from "../api/config";
import { hasDirectIdentityFile } from "./authenticationPolicy";

function detail(values: string[][]): HostDetail {
  return {
    form: {
      entry: {
        identity: { path: "config", alias: "edge" },
        file: { path: "config", absolute: "/home/tester/.ssh/config" },
        line: 1, patterns: ["edge"], editable: true,
      },
      fields: values.map((entry, index) => ({
        line: index + 2, keyword: "IdentityFile", values: entry, category: "basic", editable: true,
      })),
      raw: "Host edge\n", comment: "", commentLines: 0,
    },
    metadata: { identity: { path: "config", alias: "edge" }, favourite: false },
    effective: { alias: "edge", entries: [] },
    file: {
      file: { path: "config", absolute: "/home/tester/.ssh/config" },
      contents: "Host edge\n", digest: "digest", editable: true, exists: true,
    },
  };
}

describe("connection authentication policy", () => {
  it("ignores IdentityFile none but treats any concrete direct value as explicit", () => {
    expect(hasDirectIdentityFile(detail([]))).toBe(false);
    expect(hasDirectIdentityFile(detail([["none"]]))).toBe(false);
    expect(hasDirectIdentityFile(detail([["NONE"]]))).toBe(false);
    expect(hasDirectIdentityFile(detail([["none"], ["~/.ssh/id_work"]]))).toBe(true);
    expect(hasDirectIdentityFile(detail([["/opt/custom/key"]]))).toBe(true);
  });
});
