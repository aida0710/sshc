import { describe, expect, it } from "vitest";
import type { Translate } from "../i18n/context";
import { createConnectionFieldErrors } from "./createConnectionValidation";

const t = ((key) => key) as Translate;

describe("createConnectionFieldErrors", () => {
  it("requires a name and a host name but lets the user and the port stay empty", () => {
    expect(createConnectionFieldErrors(t, { alias: "", hostName: "", user: "", port: "" })).toEqual({
      alias: "conn.createAliasRequired",
      hostName: "conn.createHostRequired",
      user: "",
      port: "",
    });
  });

  it("refuses a user name with a space and a port outside 1 to 65535", () => {
    const errors = createConnectionFieldErrors(t, { alias: "web", hostName: "web.example", user: "a b", port: "65536" });
    expect(errors).toEqual({ alias: "", hostName: "", user: "conn.createUserInvalid", port: "conn.createPortInvalid" });
  });
});
