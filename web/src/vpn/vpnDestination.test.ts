import { describe, expect, it } from "vitest";
import destinationCases from "../../../internal/vpn/testdata/destination-cases.json";
import { vpnDestinationHostRefusal } from "./vpnDestination";

// 画面が確かめるのは接続先のホストだけなので、Go の検査と同じ表
// （internal/vpn/testdata/destination-cases.json）のうち、ポートが正しい行を使う。ポートの
// 誤りは engine だけが断る。Go の側は internal/vpn/destination_test.go が同じ表を読む。
const hostPort = /^(?:\[(?<bracketed>[^\]]*)\]|(?<plain>[^:]*)):(?<port>[0-9]+)$/;
const maxPort = 65535;

function hostWithAValidPort(address: string): string | null {
  const parts = hostPort.exec(address)?.groups;
  if (parts === undefined) return null;
  const port = Number(parts.port);
  if (port < 1 || port > maxPort) return null;
  return parts.bracketed ?? parts.plain ?? null;
}

const hostCases = destinationCases.cases.flatMap((test) => {
  const host = hostWithAValidPort(test.address);
  return host === null ? [] : [{ ...test, host }];
});

describe("vpnDestinationHostRefusal", () => {
  it("reads a table that has host cases", () => {
    expect(hostCases.length).toBeGreaterThan(0);
  });

  it.each(hostCases.map((test) => [test.name, test] as const))(
    "answers the shared table the way the engine does: %s",
    (_name, test) => {
      const refused = vpnDestinationHostRefusal(test.host, test.dns);
      expect(refused ?? "").toBe(test.reason);
    },
  );
});
