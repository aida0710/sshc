import { describe, expect, it } from "vitest";
import { openVPNSecretsFieldError } from "./openVPNSecretRules";
import { emptySecrets } from "./vpnProfileDraft";

const config = "client\nremote vpn.example.jp 1194\n";

describe("openVPNSecretsFieldError", () => {
  it("names the configuration file when none is given and none is stored", () => {
    expect(openVPNSecretsFieldError({ secrets: emptySecrets, stored: new Set(), username: "" }))
      .toEqual({ field: "secrets.openvpnConfig", reason: "required" });
  });

  it("keeps the stored configuration file when the field is left blank", () => {
    expect(openVPNSecretsFieldError({ secrets: emptySecrets, stored: new Set(["openvpnConfig"]), username: "" }))
      .toBeNull();
  });

  it("names the line and the directive that cannot be used", () => {
    expect(openVPNSecretsFieldError({
      secrets: { ...emptySecrets, openvpnConfig: `${config}up /bin/true\n` }, stored: new Set(), username: "",
    })).toEqual({ field: "secrets.openvpnConfig", reason: "runs_command", line: 3, directive: "up" });
  });

  it("asks for a username when the file asks for credentials", () => {
    expect(openVPNSecretsFieldError({
      secrets: { ...emptySecrets, openvpnConfig: `${config}auth-user-pass\n` }, stored: new Set(), username: "",
    })).toEqual({ field: "openvpn.username", reason: "required_by_config" });
  });

  it("asks for a password only when a username is given", () => {
    const secrets = { ...emptySecrets, openvpnConfig: config };
    expect(openVPNSecretsFieldError({ secrets, stored: new Set(), username: "" })).toBeNull();
    expect(openVPNSecretsFieldError({ secrets, stored: new Set(), username: "fixture" }))
      .toEqual({ field: "secrets.openvpnPassword", reason: "required" });
    expect(openVPNSecretsFieldError({ secrets: { ...secrets, openvpnPassword: "first\nsecond" }, stored: new Set(), username: "fixture" }))
      .toEqual({ field: "secrets.openvpnPassword", reason: "format" });
  });
});
