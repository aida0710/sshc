import { describe, expect, it } from "vitest";
import type { HostDetail } from "../api/config";
import { deriveBasicField } from "./basicFields";

function detail(fields: HostDetail["form"]["fields"], entries: HostDetail["effective"]["entries"]): HostDetail {
  return {
    form: {
      entry: {
        identity: { path: "config", alias: "edge" },
        file: { path: "config", absolute: "/home/tester/.ssh/config" },
        line: 1,
        patterns: ["edge"],
        editable: true,
      },
      fields,
      raw: "Host edge\n",
      comment: "",
      commentLines: 0,
    },
    metadata: { identity: { path: "config", alias: "edge" }, favourite: false },
    effective: { alias: "edge", entries },
    file: {
      file: { path: "config", absolute: "/home/tester/.ssh/config" },
      contents: "Host edge\n",
      digest: "digest",
      editable: true,
      exists: true,
    },
  };
}

describe("deriveBasicField", () => {
  it("uses the one direct directive", () => {
    const state = deriveBasicField(detail([
      { line: 2, keyword: "HostName", values: ["direct.example"], category: "basic", editable: true },
    ], [
      { keyword: "HostName", values: ["direct.example"], source: { path: "config", line: 2 } },
    ]), "HostName");

    expect(state).toEqual({
      keyword: "HostName",
      value: "direct.example",
      origin: "direct",
      source: { path: "config", line: 2 },
      editable: true,
    });
  });

  it("uses the effective source when the selected block has no direct directive", () => {
    const state = deriveBasicField(detail([], [
      { keyword: "User", values: ["inherited-user"], source: { path: "conf.d/defaults.conf", line: 8 } },
    ]), "User");

    expect(state).toEqual({
      keyword: "User",
      value: "inherited-user",
      origin: "inherited",
      source: { path: "conf.d/defaults.conf", line: 8 },
      editable: true,
    });
  });

  it.each([
    ["HostName", "edge"],
    ["User", ""],
    ["Port", "22"],
  ] as const)("provides the SSH default for %s", (keyword, value) => {
    expect(deriveBasicField(detail([], []), keyword)).toEqual({
      keyword,
      value,
      origin: "default",
      editable: true,
    });
  });

  it("keeps duplicate direct values visible but read-only", () => {
    const state = deriveBasicField(detail([
      { line: 2, keyword: "Port", values: ["22"], category: "basic", editable: true },
      { line: 3, keyword: "port", values: ["2222"], category: "basic", duplicate: true, editable: true },
    ], [
      { keyword: "Port", values: ["22"], source: { path: "config", line: 2 } },
    ]), "Port");

    expect(state).toEqual({
      keyword: "Port",
      value: "22  /  2222",
      origin: "complex",
      editable: false,
    });
  });
});
