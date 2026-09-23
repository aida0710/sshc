import { describe, expect, it } from "vitest";
import targetCases from "../../../internal/vpn/testdata/target-cases.json";
import { vpnTargetReaches } from "./vpnEndpoint";

// Go の照合と同じ表（internal/vpn/testdata/target-cases.json）に対して、画面の照合を
// 確かめる。Go の側は internal/application/vpnprofile_target_cases_test.go が同じ表を読む。
describe("vpnTargetReaches", () => {
  it("reads a table that has cases", () => {
    expect(targetCases.cases.length).toBeGreaterThan(0);
  });

  it.each(targetCases.cases.map((test) => [test.name, test] as const))(
    "answers the shared table the way the engine does: %s",
    (_name, test) => {
      expect(vpnTargetReaches(test.target, test.address)).toBe(test.reaches);
    },
  );
});
