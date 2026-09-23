import { describe, expect, it } from "vitest";
import profileCases from "../../../internal/vpn/testdata/profile-cases.json";
import type { VPNProfile } from "../api/vpn";
import { vpnProfileFieldError } from "./vpnProfileRules";

// Go の検査と同じ表（internal/vpn/testdata/profile-cases.json）に対して、画面の検査を
// 確かめる。Go の側は internal/application/vpnprofile_cases_test.go が同じ表を読む。
describe("vpnProfileFieldError", () => {
  it("reads a table that has cases", () => {
    expect(profileCases.cases.length).toBeGreaterThan(0);
  });

  it.each(profileCases.cases.map((test) => [test.name, test] as const))(
    "answers the shared table the way the engine does: %s",
    (_name, test) => {
      const refused = vpnProfileFieldError(test.profile as VPNProfile);
      if (test.field === "") {
        expect(refused).toBeNull();
        return;
      }
      const limit = "limit" in test ? test.limit : undefined;
      expect(refused).toEqual(
        limit === undefined
          ? { field: test.field, reason: test.reason }
          : { field: test.field, reason: test.reason, limit },
      );
    },
  );
});
