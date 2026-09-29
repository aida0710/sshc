import { describe, expect, it } from "vitest";
import configCases from "../../../internal/vpn/testdata/wireguard-config-cases.json";
import { inspectWireGuardConfig, wireGuardServers } from "./wireGuardConfig";

// Go の検査と同じ表（internal/vpn/testdata/wireguard-config-cases.json）に対して、画面の検査を
// 確かめる。Go の側は internal/vpn/wireguard_config_test.go が同じ表を読む。
describe("inspectWireGuardConfig", () => {
  it("reads a table that has cases", () => {
    expect(configCases.cases.length).toBeGreaterThan(0);
  });

  it.each(configCases.cases.map((test) => [test.name, test] as const))(
    "answers the shared table the way the engine does: %s",
    (_name, test) => {
      const inspected = inspectWireGuardConfig(`${test.config.join("\n")}\n`);
      if (test.reason !== "") {
        const limit = "limit" in test ? test.limit : undefined;
        expect(inspected).toEqual({
          refusal: {
            field: "secrets.wireguardConfig",
            reason: test.reason,
            ...(test.line === 0 ? {} : { line: test.line }),
            ...(test.directive === "" ? {} : { directive: test.directive }),
            ...(limit === undefined ? {} : { limit }),
          },
        });
        return;
      }
      if (!("config" in inspected)) throw new Error(`refused: ${JSON.stringify(inspected.refusal)}`);
      const { config } = inspected;
      expect(config.addresses).toEqual("addresses" in test ? test.addresses : []);
      expect(config.dns).toEqual("dns" in test ? test.dns : []);
      expect(wireGuardServers(config)).toEqual("servers" in test ? test.servers : []);
    },
  );
});
