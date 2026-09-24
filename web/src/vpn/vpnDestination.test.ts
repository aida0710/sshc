import { describe, expect, it } from "vitest";
import destinationCases from "../../../internal/vpn/testdata/destination-cases.json";
import { vpnDestinationRefusal } from "./vpnDestination";

// Go の検査と同じ表（internal/vpn/testdata/destination-cases.json）に対して、画面の検査を
// 確かめる。Go の側は internal/vpn/destination_test.go が同じ表を読む。
describe("vpnDestinationRefusal", () => {
  it("reads a table that has cases", () => {
    expect(destinationCases.cases.length).toBeGreaterThan(0);
  });

  it.each(destinationCases.cases.map((test) => [test.name, test] as const))(
    "answers the shared table the way the engine does: %s",
    (_name, test) => {
      const refused = vpnDestinationRefusal(test.address, test.dns);
      expect(refused ?? "").toBe(test.reason);
    },
  );
});
