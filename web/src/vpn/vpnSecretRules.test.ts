import { describe, expect, it } from "vitest";
import secretCases from "../../../internal/vpn/testdata/secret-cases.json";
import type { VPNProfile } from "../api/vpn";
import { emptySecrets } from "./vpnProfileDraft";
import { type VPNSecretKey, vpnSecretsFieldError } from "./vpnSecretRules";

// SecretCase は、Go の保存の経路と共有する表（internal/vpn/testdata/secret-cases.json）の1行である。
type SecretCase = {
  name: string;
  profile: VPNProfile;
  secrets: Partial<Record<VPNSecretKey, string>>;
  stored?: Partial<Record<VPNSecretKey, string>>;
  field: string;
  reason: string;
  limit?: number;
  engineOnly?: boolean;
};

const sharedSecretCases = secretCases.cases as SecretCase[];

// Go の検査と同じ表に対して、画面の検査を確かめる。Go の側は
// internal/vpnprofile/secret_cases_test.go が同じ表を読む。engineOnly の行は、画面では断らない。
describe("vpnSecretsFieldError on the shared table", () => {
  it("reads a table that has cases", () => {
    expect(sharedSecretCases.length).toBeGreaterThan(0);
  });

  it.each(sharedSecretCases.map((test) => [test.name, test] as const))(
    "answers the shared table the way the engine does: %s",
    (_name, test) => {
      const refused = vpnSecretsFieldError({
        backend: test.profile.backend,
        secondFactor: test.profile.openconnect?.secondFactor ?? "",
        username: test.profile.openvpn?.username ?? "",
        ikev2Authentication: test.profile.ikev2?.authentication ?? "",
        secrets: { ...emptySecrets, ...test.secrets },
        stored: new Set(Object.keys(test.stored ?? {}) as VPNSecretKey[]),
      });
      const expected = test.engineOnly || test.field === ""
        ? null
        : { field: test.field, reason: test.reason, ...(test.limit === undefined ? {} : { limit: test.limit }) };
      expect(refused).toEqual(expected);
    },
  );
});

describe("vpnSecretsFieldError", () => {
  it("asks for every secret the type needs when nothing is stored yet", () => {
    expect(vpnSecretsFieldError({
      backend: "l2tp_ipsec", secondFactor: "", secrets: { ...emptySecrets, l2tpPassword: "a password" }, stored: new Set(),
    })).toEqual({ field: "secrets.ipsecPsk", reason: "required" });
  });

  it("sends WireGuard to the rules of its configuration file", () => {
    expect(vpnSecretsFieldError({
      backend: "wireguard", secondFactor: "", secrets: emptySecrets, stored: new Set(),
    })).toEqual({ field: "secrets.wireguardConfig", reason: "required" });
    expect(vpnSecretsFieldError({
      backend: "wireguard", secondFactor: "", secrets: { ...emptySecrets, wireguardConfig: "[Interface]\nPostUp = echo\n" },
      stored: new Set(),
    })).toEqual({ field: "secrets.wireguardConfig", reason: "runs_command", line: 2, directive: "PostUp" });
  });

  it("keeps a stored WireGuard configuration file when its field was left blank", () => {
    expect(vpnSecretsFieldError({
      backend: "wireguard", secondFactor: "", secrets: emptySecrets, stored: new Set(["wireguardConfig"]),
    })).toBeNull();
  });

  it("asks for a TOTP secret once the second factor uses one, even if a password is stored", () => {
    expect(vpnSecretsFieldError({
      backend: "openconnect", secondFactor: "totp", secrets: emptySecrets, stored: new Set(["openconnectPassword"]),
    })).toEqual({ field: "secrets.openconnectTotpSecret", reason: "required" });
  });

  it("refuses a TOTP secret longer than the API accepts", () => {
    expect(vpnSecretsFieldError({
      backend: "openconnect",
      secondFactor: "totp",
      secrets: { ...emptySecrets, openconnectPassword: "a password", openconnectTotpSecret: "A".repeat(513) },
      stored: new Set(),
    })).toEqual({ field: "secrets.openconnectTotpSecret", reason: "too_long", limit: 512 });
  });

  it("asks IKEv2 for the secret of the chosen authentication only", () => {
    expect(vpnSecretsFieldError({
      backend: "ikev2", secondFactor: "", ikev2Authentication: "eap-mschapv2", secrets: emptySecrets, stored: new Set(),
    })).toEqual({ field: "secrets.ikev2Password", reason: "required" });
    expect(vpnSecretsFieldError({
      backend: "ikev2", secondFactor: "", ikev2Authentication: "psk", secrets: { ...emptySecrets, ikev2Password: "a password" },
      stored: new Set(),
    })).toEqual({ field: "secrets.ikev2Psk", reason: "required" });
  });

  it("does not let a stored IKEv2 password stand in for a pre-shared key", () => {
    expect(vpnSecretsFieldError({
      backend: "ikev2", secondFactor: "", ikev2Authentication: "psk", secrets: emptySecrets, stored: new Set(["ikev2Password"]),
    })).toEqual({ field: "secrets.ikev2Psk", reason: "required" });
  });
});
