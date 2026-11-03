import { describe, expect, it } from "vitest";
import type { Translate } from "../i18n/context";
import { createConnectionFieldErrors } from "./createConnectionValidation";

const t = ((key) => key) as Translate;

describe("createConnectionFieldErrors", () => {
  it("requires a host name but lets the name, the user and the port stay empty", () => {
    expect(createConnectionFieldErrors(t, { alias: "", hostName: "", user: "", port: "" }, "")).toEqual({
      alias: "",
      hostName: "conn.createHostRequired",
      user: "",
      port: "",
    });
  });

  it("takes an empty name from what the host name suggests, and asks for one when it suggests none", () => {
    const fields = { alias: "", hostName: "10.0.0.5", user: "", port: "" };
    expect(createConnectionFieldErrors(t, fields, "10-0-0-5").alias).toBe("");
    expect(createConnectionFieldErrors(t, fields, "").alias).toBe("conn.createAliasRequired");
  });

  it("says nothing about the name while the host name is what is wrong", () => {
    const errors = createConnectionFieldErrors(t, { alias: "", hostName: "bad host", user: "", port: "" }, "");
    expect(errors.alias).toBe("");
    expect(errors.hostName).toBe("conn.createHostInvalid");
  });

  it("refuses a user name with a space and a port outside 1 to 65535", () => {
    const errors = createConnectionFieldErrors(t, { alias: "web", hostName: "web.example", user: "a b", port: "65536" }, "");
    expect(errors).toEqual({ alias: "", hostName: "", user: "conn.createUserInvalid", port: "conn.createPortInvalid" });
  });
});
