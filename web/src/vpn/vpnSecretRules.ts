import type { VPNSecrets } from "../api/vpn";
import type { VPNBackend } from "./vpnBackends";
import type { VPNFieldError } from "./vpnFieldErrors";
import { vpnWireGuardKeyError } from "./vpnProfileRules";

// VPN プロファイルのシークレットを、送る前に項目ごとに確かめる。
//
// 未入力と WireGuard の鍵の形は engine（Go の validateSecrets）の規則の写しで、検査の
// 順番も同じにする。長さの上限は API（api/openapi.yaml の VPNSecrets）の写しである。
// engine は長さを見ないが、送る前の検査（validateAPIRequest）は上限を超えた値を項目の
// 名前なしで断るので、その前にここで項目ごとの理由を出す。送る前の検査と同じく、
// 長さは UTF-16 の長さで数える。

export type VPNSecretKey = keyof VPNSecrets;

// 秘密鍵以外のシークレットの上限である（API と同じ値）。
const maxPasswordLength = 256;
const maxTOTPSecretLength = 512;

const secretLimits: Partial<Record<VPNSecretKey, number>> = {
  l2tpPassword: maxPasswordLength,
  ipsecPsk: maxPasswordLength,
  openconnectPassword: maxPasswordLength,
  openconnectTotpSecret: maxTOTPSecretLength,
};

// vpnSecretKeys は、その方式と二要素認証の答え方が要るシークレットを、engine が
// 確かめる順に返す。
export function vpnSecretKeys(backend: VPNBackend, secondFactor: string): VPNSecretKey[] {
  switch (backend) {
    case "wireguard":
      return ["wireguardPrivateKey"];
    case "l2tp_ipsec":
      return ["l2tpPassword", "ipsecPsk"];
    case "openconnect":
      return secondFactor === "totp" ? ["openconnectPassword", "openconnectTotpSecret"] : ["openconnectPassword"];
  }
}

export type VPNSecretsDraft = {
  backend: VPNBackend;
  secondFactor: string;
  secrets: Required<VPNSecrets>;
  // stored は、保存済みで、空欄なら engine がそのまま使うシークレットである。新しく
  // 作るときは空である。
  stored: ReadonlySet<VPNSecretKey>;
};

// vpnSecretsFieldError は、送れないシークレットがあれば、最初の項目と理由を返す。
export function vpnSecretsFieldError({ backend, secondFactor, secrets, stored }: VPNSecretsDraft): VPNFieldError | null {
  for (const key of vpnSecretKeys(backend, secondFactor)) {
    const field = `secrets.${key}`;
    const value = secrets[key];
    if (value === "") {
      if (stored.has(key)) continue;
      return { field, reason: "required" };
    }
    if (key === "wireguardPrivateKey") {
      const refused = vpnWireGuardKeyError(field, value);
      if (refused !== null) return refused;
      continue;
    }
    const limit = secretLimits[key];
    if (limit !== undefined && value.length > limit) return { field, reason: "too_long", limit };
  }
  return null;
}
