import { describe, expect, it } from "vitest";
import corpus from "../rules/corpus.generated.json";
import { isDirectDestination, parseDirectDestination, sshSessionTarget } from "./directDestination";

describe("parseDirectDestination", () => {
  it.each([
    ["deploy@10.0.0.5", { user: "deploy", host: "10.0.0.5" }],
    ["deploy@10.0.0.5:2222", { user: "deploy", host: "10.0.0.5", port: 2222 }],
    ["  deploy@web.example.com ", { text: "deploy@web.example.com", user: "deploy", host: "web.example.com" }],
    ["deploy@2001:db8::1", { user: "deploy", host: "2001:db8::1" }],
    ["deploy@[2001:db8::1]:2222", { user: "deploy", host: "2001:db8::1", port: 2222 }],
    ["user@corp.example@web", { user: "user@corp.example", host: "web" }],
  ])("reads %s", (raw, expected) => {
    expect(parseDirectDestination(raw)).toMatchObject(expected);
  });

  it("does not take a search for a destination", () => {
    for (const search of ["web", "deploy", "10.0.0.5", "web:22", "deploy@"]) {
      expect(parseDirectDestination(search)).toBeNull();
    }
  });

  it.each([..."'`\";&<>|(){}$\\"])("refuses %s in the user, which a shell would read", (character) => {
    expect(parseDirectDestination(`de${character}ploy@host`)).toBeNull();
  });

  describe("agrees with the engine", () => {
    for (const item of corpus.directDestination) {
      it(`${JSON.stringify(item.input)}: ${item.why}`, () => {
        expect(parseDirectDestination(item.input) !== null).toBe(item.valid);
      });
    }
  });
});

describe("sshSessionTarget", () => {
  it("asks for a connection by its name and for anything else as a destination", () => {
    expect(sshSessionTarget("web-01")).toEqual({ alias: "web-01" });
    expect(sshSessionTarget("deploy@10.0.0.5")).toEqual({ destination: "deploy@10.0.0.5" });
  });
});

describe("isDirectDestination", () => {
  it("tells a destination from a connection name", () => {
    expect(isDirectDestination("deploy@10.0.0.5")).toBe(true);
    expect(isDirectDestination("web-01")).toBe(false);
  });
});
