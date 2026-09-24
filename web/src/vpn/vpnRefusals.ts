import type { MessageKey } from "../i18n/messages";

// engine が VPN の操作や接続を断った理由（Go の internal/vpnrefusal の語）と、その画面での
// 言い方。文は Go の vpnrefusal.Sentence と同じ言い方に揃える。

// vpnRefusalMessages は、理由の語を持たない拒否の言い方である。項目を名指しする
// vpn_profile_invalid と vpn_secrets_missing は、項目が分からないときにこの文を使う。
export const vpnRefusalMessages = {
  vpn_profile_unknown: "vpn.profileUnknown",
  vpn_profile_exists: "vpn.profileExists",
  connection_unknown: "vpn.connectionUnknown",
  vpn_profile_invalid: "vpn.profileInvalid",
  vpn_secrets_missing: "vpn.secretsMissing",
  vault_locked: "vpn.vaultLocked",
  vault_missing: "vpn.vaultMissing",
  vpn_docker_missing: "vpn.dockerMissing",
  vpn_docker_not_running: "vpn.dockerNotRunning",
  vpn_tunnel_device_missing: "vpn.tunnelDeviceMissing",
  vpn_image_build_failed: "vpn.imageBuildFailed",
  vpn_container_foreign: "vpn.containerForeign",
  vpn_socket_path_too_long: "vpn.socketPathTooLong",
  vpn_changed_concurrently: "vpn.changedConcurrently",
} as const satisfies Record<string, MessageKey>;

// 理由の語（problem の reason）で言い方が決まる拒否である。言い方は vpnFailureReasons.ts にある。
const vpnReasonedRefusals = ["vpn_destination_invalid", "vpn_target_failed", "vpn_session_failed"];

// vpnProblemCodes は、engine が VPN の拒否として返す code のすべてである（Go の
// vpnrefusal.Known と同じ）。これらは共通の失敗通知に回さず、受け取った画面が説明する。
export const vpnProblemCodes: readonly string[] = [...Object.keys(vpnRefusalMessages), ...vpnReasonedRefusals];
