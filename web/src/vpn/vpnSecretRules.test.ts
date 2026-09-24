import { describe, expect, it } from "vitest";
import { emptySecrets } from "./vpnProfileDraft";
import { vpnSecretsFieldError } from "./vpnSecretRules";

const privateKey = "aAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAA=";

describe("vpnSecretsFieldError", () => {
  it("asks for every secret the type needs when nothing is stored yet", () => {
    expect(vpnSecretsFieldError({
      backend: "l2tp_ipsec", secondFactor: "", secrets: { ...emptySecrets, l2tpPassword: "a password" }, stored: new Set(),
    })).toEqual({ field: "secrets.ipsecPsk", reason: "required" });
  });

  it("lets a blank secret keep the stored value", () => {
    expect(vpnSecretsFieldError({
      backend: "wireguard", secondFactor: "", secrets: emptySecrets, stored: new Set(["wireguardPrivateKey"]),
    })).toBeNull();
  });

  it("checks the form of a WireGuard key that was typed", () => {
    expect(vpnSecretsFieldError({
      backend: "wireguard", secondFactor: "", secrets: { ...emptySecrets, wireguardPrivateKey: `${privateKey}=` }, stored: new Set(),
    })).toEqual({ field: "secrets.wireguardPrivateKey", reason: "format" });
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
});
