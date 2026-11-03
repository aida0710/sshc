import { describe, expect, it } from "vitest";
import { maxAliasLength } from "../rules/generated";
import { isValidAlias } from "../rules/rules";
import { connectionAliasFromHostName, uniqueConnectionAlias } from "./connectionAliasFromHost";

describe("connectionAliasFromHostName", () => {
  it.each([
    ["10.0.0.5", "10-0-0-5"],
    ["web.example.com", "web"],
    ["localhost", "localhost"],
    ["web-01.example.com", "web-01"],
    ["2001:db8::1", "2001-db8--1"],
    ["::1", "1"],
  ])("suggests a name for %s", (hostName, alias) => {
    expect(connectionAliasFromHostName(hostName)).toBe(alias);
    expect(isValidAlias(alias)).toBe(true);
  });

  it("suggests nothing for a host name with no valid name in it", () => {
    expect(connectionAliasFromHostName("")).toBe("");
    expect(connectionAliasFromHostName("::")).toBe("");
  });

  it("stays within the limit of a connection name", () => {
    expect(connectionAliasFromHostName(`${"a".repeat(100)}.example.com`)).toHaveLength(maxAliasLength);
  });
});

describe("uniqueConnectionAlias", () => {
  it("keeps a name nobody has", () => {
    expect(uniqueConnectionAlias("web", ["db"])).toBe("web");
  });

  it("numbers a name that is taken, whatever its case", () => {
    expect(uniqueConnectionAlias("web", ["WEB"])).toBe("web-2");
    expect(uniqueConnectionAlias("web", ["web", "web-2"])).toBe("web-3");
  });

  it("keeps the number within the limit of a connection name", () => {
    const long = "a".repeat(maxAliasLength);
    const unique = uniqueConnectionAlias(long, [long]);
    expect(unique).toHaveLength(maxAliasLength);
    expect(unique.endsWith("-2")).toBe(true);
  });

  it("gives nothing for nothing", () => {
    expect(uniqueConnectionAlias("", [])).toBe("");
  });
});
