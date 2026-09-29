import { describe, expect, it } from "vitest";
import configCases from "../../../internal/vpn/testdata/openvpn-config-cases.json";
import { inspectOpenVPNConfig } from "./openVPNConfig";

// Go の検査と同じ表（internal/vpn/testdata/openvpn-config-cases.json）に対して、画面の検査を
// 確かめる。Go の側は internal/vpn/openvpn_config_test.go が同じ表を読む。
describe("inspectOpenVPNConfig", () => {
  it("reads a table that has cases", () => {
    expect(configCases.cases.length).toBeGreaterThan(0);
  });

  it.each(configCases.cases.map((test) => [test.name, test] as const))(
    "answers the shared table the way the engine does: %s",
    (_name, test) => {
      const inspected = inspectOpenVPNConfig(`${test.config.join("\n")}\n`);
      if (test.reason === "") {
        expect(inspected).toEqual({
          summary: { servers: "servers" in test ? test.servers : [], asksCredentials: "asksCredentials" in test && test.asksCredentials },
        });
        return;
      }
      const limit = "limit" in test ? test.limit : undefined;
      expect(inspected).toEqual({
        refusal: {
          field: "secrets.openvpnConfig",
          reason: test.reason,
          line: test.line,
          ...(test.directive === "" ? {} : { directive: test.directive }),
          ...(limit === undefined ? {} : { limit }),
        },
      });
    },
  );

  it("does not take a directive named after an object property for a listed one", () => {
    expect(inspectOpenVPNConfig("client\nremote vpn.example.jp\nconstructor x\n<toString>\n</toString>\n")).toEqual({
      refusal: { field: "secrets.openvpnConfig", reason: "unsupported_inline", line: 4 },
    });
  });
});
