import { describe, expect, it } from "vitest";
import failureReasons from "../../../internal/vpnrefusal/testdata/failure-reasons.json";
import { vpnRouteFailureMessage, vpnTargetFailureMessage } from "./vpnFailureReasons";

// engine の理由の語の表（internal/vpnrefusal/testdata/failure-reasons.json）に対して、画面が
// どの語にも言い方を持つことを確かめる。Go の側は internal/vpnrefusal/vpnrefusal_test.go が
// 同じ表を読む。
describe("vpnFailureReasons", () => {
  it("reads a table that has reasons", () => {
    expect(failureReasons.route.length).toBeGreaterThan(0);
    expect(failureReasons.target.length).toBeGreaterThan(0);
  });

  it.each(failureReasons.route.filter((reason) => reason !== "unknown"))(
    "has a message for the route failure %s",
    (reason) => {
      expect(vpnRouteFailureMessage(reason)).not.toBe("vpn.failure.unknown");
    },
  );

  it.each(failureReasons.target)("has a message for the target failure %s", (reason) => {
    expect(vpnTargetFailureMessage(reason)).not.toBe("vpn.failure.unknown");
  });
});
