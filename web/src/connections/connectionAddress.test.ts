import { describe, expect, it } from "vitest";
import type { HostEntry } from "../api/config";
import { addressesByAlias, connectionAddress } from "./connectionAddress";

describe("connectionAddress", () => {
  it("leaves out the default port 22", () => {
    expect(connectionAddress({ hostName: "198.51.100.11", user: "ops", port: "22" })).toBe("ops@198.51.100.11");
  });

  it("keeps any other port", () => {
    expect(connectionAddress({ hostName: "203.0.113.10", user: "ops", port: "2222" })).toBe("ops@203.0.113.10:2222");
  });

  it("puts an IPv6 address in brackets", () => {
    expect(connectionAddress({ hostName: "2001:db8::1", port: "2200" })).toBe("[2001:db8::1]:2200");
  });

  it("gives nothing when there is no host", () => {
    expect(connectionAddress({ hostName: "", user: "ops" })).toBe("");
  });
});

describe("addressesByAlias", () => {
  const host = (fields: Partial<HostEntry> & { alias: string }) =>
    ({ identity: { path: "config", alias: fields.alias }, ...fields }) as unknown as HostEntry;

  it("gives each alias its address", () => {
    const addresses = addressesByAlias([host({ alias: "web-01", hostName: "198.51.100.11", user: "deploy", port: "2222" })]);
    expect(addresses.get("web-01")).toBe("deploy@198.51.100.11:2222");
  });

  it("takes the alias for the host name of a connection without one, as OpenSSH does", () => {
    expect(addressesByAlias([host({ alias: "web-01", user: "deploy" })]).get("web-01")).toBe("deploy@web-01");
  });
});
