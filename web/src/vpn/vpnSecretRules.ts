import type { VPNSecrets } from "../api/vpn";
import type { VPNBackend } from "./vpnBackends";
import type { VPNFieldError } from "./vpnFieldErrors";
import { openVPNSecretsFieldError } from "./openVPNSecretRules";
import { utf8Length } from "./utf8Length";
import { wireGuardSecretsFieldError } from "./wireGuardSecretRules";

// VPN プロファイルのシークレットを、送る前に項目ごとに確かめる。
//
// 規則の正本は engine（Go の各方式の validateSecrets）にあり、ここはその写しである。検査の
// 順番も同じにする。同じ入力に同じ項目と理由を返すことを、internal/vpn/testdata/secret-cases.json
// に対するテストで保つ。OpenVPN と WireGuard の設定ファイルは、中身まで openVPNSecretRules.ts と
// wireGuardSecretRules.ts が確かめる。
//
// 長さの上限は engine と API（api/openapi.yaml の VPNSecrets）と同じ値である。engine は UTF-8 の
// バイト数で数えるので、ここもバイト数で数える。API の maxLength（文字数で数える）より厳しいか
// 同じなので、ここを通った値は送る前の検査でも断られない。
//
// OpenConnect の TOTP の種の形（base32 または otpauth の URI）は、engine（internal/totp）だけが
// 確かめる。engine の断りは、ほかの項目と同じく欄ごとの理由として出る。

// VPNSecretKey は、フォームの欄ひとつに入力するシークレットである。
export type VPNSecretKey = keyof VPNSecrets;

// 設定ファイル以外のシークレットの上限（UTF-8 のバイト数）である（engine と API と同じ値）。
const maxPasswordLength = 256;
const maxTOTPSecretLength = 512;

const secretLimits: Partial<Record<VPNSecretKey, number>> = {
  l2tpPassword: maxPasswordLength,
  ipsecPsk: maxPasswordLength,
  openconnectPassword: maxPasswordLength,
  openconnectTotpSecret: maxTOTPSecretLength,
  ikev2Password: maxPasswordLength,
  ikev2Psk: maxPasswordLength,
};

// VPNSecretChoice は、どのシークレットが要るかを決める設定である。方式のほかに、
// OpenConnect では二要素認証の答え方、OpenVPN ではユーザー名の有無、IKEv2 では認証の
// 方式で変わる。
export type VPNSecretChoice = {
  backend: VPNBackend;
  secondFactor: string;
  // username は、OpenVPN で auth-user-pass に送るユーザー名である。ほかの方式では使わない。
  username?: string;
  // ikev2Authentication は、IKEv2 の認証の方式である。無ければ EAP として扱う。
  ikev2Authentication?: string;
};

// vpnSecretKeys は、選んだ設定が要るシークレットを、engine が確かめる順に返す。
export function vpnSecretKeys({ backend, secondFactor, username = "", ikev2Authentication }: VPNSecretChoice): VPNSecretKey[] {
  switch (backend) {
    case "wireguard":
      return ["wireguardConfig"];
    case "l2tp_ipsec":
      return ["l2tpPassword", "ipsecPsk"];
    case "openconnect":
      return secondFactor === "totp" ? ["openconnectPassword", "openconnectTotpSecret"] : ["openconnectPassword"];
    case "openvpn":
      // パスワードは、ユーザー名を送るときだけ使う。
      return username === "" ? ["openvpnConfig"] : ["openvpnConfig", "openvpnPassword"];
    case "ikev2":
      return ikev2Authentication === "psk" ? ["ikev2Psk"] : ["ikev2Password"];
  }
}

export type VPNSecretsDraft = VPNSecretChoice & {
  secrets: Record<VPNSecretKey, string>;
  // stored は、保存済みで、空欄なら engine がそのまま使うシークレットである。新しく
  // 作るときと、保存済みのシークレットを取り出してフォームに入れたときは空である。
  stored: ReadonlySet<VPNSecretKey>;
};

// vpnSecretsFieldError は、送れないシークレットがあれば、最初の項目と理由を返す。
export function vpnSecretsFieldError({ secrets, stored, ...choice }: VPNSecretsDraft): VPNFieldError | null {
  // OpenVPN と WireGuard の設定ファイルは、中身の指示まで確かめる。
  if (choice.backend === "openvpn") return openVPNSecretsFieldError({ secrets, stored, username: choice.username ?? "" });
  if (choice.backend === "wireguard") return wireGuardSecretsFieldError({ secrets, stored });
  for (const key of vpnSecretKeys(choice)) {
    const field = `secrets.${key}`;
    const value = secrets[key];
    if (value === "") {
      if (stored.has(key)) continue;
      return { field, reason: "required" };
    }
    const limit = secretLimits[key];
    if (limit !== undefined && utf8Length(value) > limit) return { field, reason: "too_long", limit };
  }
  return null;
}
