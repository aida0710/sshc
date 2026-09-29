import type { VPNSecrets } from "../api/vpn";
import { inspectOpenVPNConfig } from "./openVPNConfig";
import type { VPNFieldError } from "./vpnFieldErrors";

// OpenVPN のシークレット（設定ファイルとパスワード）を、送る前に engine と同じ順番で確かめる。
//
// 規則の正本は Go（internal/vpn/openvpn.go の validateSecrets）にある。長さの上限は API
// （api/openapi.yaml の VPNSecrets）の写しで、送る前の検査と同じく UTF-16 の長さで数える。

// maxConfigLength は、設定ファイルの長さの上限である（API と同じ値）。
const maxConfigLength = 65536;
// maxPasswordLength は、パスワードの長さの上限である（API と同じ値）。
const maxPasswordLength = 256;
// credentialForbidden は、パスワードに使えない字である。OpenVPN は1行目をユーザー名、
// 2行目をパスワードとして読む。
const credentialForbidden = /[\r\n\0]/;

export type OpenVPNSecretsDraft = {
  secrets: Required<VPNSecrets>;
  // stored は、保存済みで、空欄なら engine がそのまま使うシークレットである。
  stored: ReadonlySet<keyof VPNSecrets>;
  username: string;
};

// openVPNSecretsFieldError は、送れないシークレットがあれば、最初の項目と理由を返す。
// 設定ファイルが auth-user-pass を含むのにユーザー名が無いときも、ここで断る。
export function openVPNSecretsFieldError({ secrets, stored, username }: OpenVPNSecretsDraft): VPNFieldError | null {
  const config = secrets.openvpnConfig;
  if (config === "") {
    if (!stored.has("openvpnConfig")) return { field: "secrets.openvpnConfig", reason: "required" };
  } else {
    if (config.length > maxConfigLength) {
      return { field: "secrets.openvpnConfig", reason: "too_long", limit: maxConfigLength };
    }
    const inspected = inspectOpenVPNConfig(config);
    if ("refusal" in inspected) return inspected.refusal;
    if (inspected.summary.asksCredentials && username === "") {
      return { field: "openvpn.username", reason: "required_by_config" };
    }
  }
  if (username === "") return null;
  const password = secrets.openvpnPassword;
  if (password === "") {
    return stored.has("openvpnPassword") ? null : { field: "secrets.openvpnPassword", reason: "required" };
  }
  if (password.length > maxPasswordLength) {
    return { field: "secrets.openvpnPassword", reason: "too_long", limit: maxPasswordLength };
  }
  return credentialForbidden.test(password) ? { field: "secrets.openvpnPassword", reason: "format" } : null;
}
