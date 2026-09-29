import type { VPNFieldError } from "./vpnFieldErrors";
import type { VPNSecretKey } from "./vpnSecretRules";
import { inspectWireGuardConfig } from "./wireGuardConfig";

// WireGuard のシークレット（鍵を含む設定ファイル）を、送る前に engine と同じ規則で確かめる。
//
// 規則の正本は Go（internal/vpn/wireguard.go の validateSecrets と、wireguard_config.go の
// ParseWireGuardConfig）にある。サーバーと DNS は、この設定ファイルから読んでプロファイルに
// 入れるので、食い違いはここでは起きない。

export type WireGuardSecretsDraft = {
  secrets: Record<VPNSecretKey, string>;
  // stored は、保存済みで、空欄なら engine がそのまま使うシークレットである。
  stored: ReadonlySet<VPNSecretKey>;
};

// wireGuardSecretsFieldError は、送れない設定ファイルなら、最初の行と理由を返す。
export function wireGuardSecretsFieldError({ secrets, stored }: WireGuardSecretsDraft): VPNFieldError | null {
  const config = secrets.wireguardConfig;
  if (config === "") {
    return stored.has("wireguardConfig") ? null : { field: "secrets.wireguardConfig", reason: "required" };
  }
  const inspected = inspectWireGuardConfig(config);
  return "refusal" in inspected ? inspected.refusal : null;
}
