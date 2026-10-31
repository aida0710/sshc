import { describe, expect, it } from "vitest";
import type { HostDetail } from "../api/config";
import { directIdentityFields, isConcreteIdentityValue } from "./authenticationPolicy";

function detail(values: string[][], keyword = "IdentityFile"): HostDetail {
  return {
    form: {
      entry: {
        identity: { path: "config", alias: "edge" },
        file: { path: "config", absolute: "/home/tester/.ssh/config" },
        line: 1, patterns: ["edge"], editable: true,
      },
      fields: values.map((entry, index) => ({
        line: index + 2, keyword, values: entry, category: "basic", editable: true,
      })),
      raw: "Host edge\n", comment: "", commentLines: 0,
    },
    metadata: { identity: { path: "config", alias: "edge" } },
    effective: { alias: "edge", entries: [] },
    file: {
      file: { path: "config", absolute: "/home/tester/.ssh/config" },
      contents: "Host edge\n", digest: "digest", editable: true, exists: true,
    },
  };
}

describe("connection authentication policy", () => {
  it("finds the IdentityFile lines whatever their case", () => {
    expect(directIdentityFields(detail([["~/.ssh/id_work"]], "identityfile"))).toHaveLength(1);
    expect(directIdentityFields(detail([["~/.ssh/id_work"]], "CertificateFile"))).toEqual([]);
  });

  it("ignores IdentityFile none but treats any concrete value as explicit", () => {
    expect(isConcreteIdentityValue("")).toBe(false);
    expect(isConcreteIdentityValue("none")).toBe(false);
    expect(isConcreteIdentityValue(" NONE ")).toBe(false);
    expect(isConcreteIdentityValue("~/.ssh/id_work")).toBe(true);
    expect(isConcreteIdentityValue("/opt/custom/key")).toBe(true);
  });
});
