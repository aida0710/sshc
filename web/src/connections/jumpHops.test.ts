import { describe, expect, it } from "vitest";
import type { HostEntry } from "../api/config";
import { connectionsJumpingThrough, jumpHopNames, unsavedJumpHops } from "./jumpHops";

function host(alias: string, proxyJump?: string): HostEntry {
  return { identity: { path: "config", alias }, file: { path: "config", absolute: "/home/me/.ssh/config" }, line: 1, patterns: [alias], editable: true, ...(proxyJump === undefined ? {} : { proxyJump }) };
}

describe("jumpHopNames", () => {
  it("reads the connection of every hop, with or without a user, a port or ssh://", () => {
    expect(jumpHopNames("ops@bastion:2222,nas,ssh://dev@edge:22")).toEqual(["bastion", "nas", "edge"]);
  });
  it("leaves out an IPv6 address in brackets and none", () => {
    expect(jumpHopNames("[2001:db8::1]:22,none")).toEqual([]);
  });
});

describe("connectionsJumpingThrough", () => {
  it("lists the connections that go through a deleted connection, in any case, but not the deleted ones themselves", () => {
    const hosts = [host("jump"), host("target-b", "JUMP"), host("chain", "nas,jump:2222"), host("direct"), host("also-deleted", "jump")];
    expect(connectionsJumpingThrough(hosts, ["jump", "also-deleted"]).map((entry) => entry.identity.alias)).toEqual(["target-b", "chain"]);
  });
});

describe("unsavedJumpHops", () => {
  it("names a hop that looks like a connection but is not saved", () => {
    expect(unsavedJumpHops("jump,nas", ["nas", "target-b"])).toEqual(["jump"]);
  });
  it("leaves out a DNS name or an address, which is meant as a host name", () => {
    expect(unsavedJumpHops("bastion.example.com,203.0.113.5", ["nas"])).toEqual([]);
  });
});
