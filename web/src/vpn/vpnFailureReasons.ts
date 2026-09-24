import type { MessageKey } from "../i18n/messages";
import type { VPNDestinationReason } from "./vpnDestination";

// 理由の語（problem の reason）を持つ VPN の拒否と、その理由の画面での言い方。engine の
// vpn.FailureReason と vpn.Reason と同じ語を使う。文は Go の internal/vpnrefusal と同じ
// 言い方に揃える。

// 経路を用意できなかった理由（vpn_session_failed）である。
const sessionFailureMessages: Record<string, MessageKey> = {
  unknown: "vpn.failure.unknown",
  timeout: "vpn.failure.timeout",
  server_unresolved: "vpn.failure.server_unresolved",
  ipsec_negotiation: "vpn.failure.ipsec_negotiation",
  ppp_authentication: "vpn.failure.ppp_authentication",
  openconnect_failed: "vpn.failure.openconnect_failed",
  handshake_timeout: "vpn.failure.handshake_timeout",
  tunnel_lost: "vpn.failure.tunnel_lost",
};

// 経路はあるが、VPN 経由で接続先へ接続できなかった理由（vpn_target_failed）である。
const targetFailureMessages: Record<string, MessageKey> = {
  target_unresolved: "vpn.targetFailure.target_unresolved",
  target_needs_dns: "vpn.destination.name_needs_dns",
  target_is_server: "vpn.targetFailure.target_is_server",
  target_unreachable: "vpn.targetFailure.target_unreachable",
  tunnel_lost: "vpn.targetFailure.tunnel_lost",
  timeout: "vpn.failure.timeout",
};

// 接続先（HostName と Port）を VPN 経由では使えない理由（vpn_destination_invalid）である。
const destinationMessages: Record<VPNDestinationReason, MessageKey> = {
  format: "vpn.destination.format",
  out_of_range: "vpn.destination.out_of_range",
  not_ipv4: "vpn.destination.not_ipv4",
  unroutable: "vpn.destination.unroutable",
  name_needs_dns: "vpn.destination.name_needs_dns",
};

// 理由の語が無いか、この画面の知らない語なら、原因を特定できなかったとしてログへ案内する。
export function vpnSessionFailureMessage(reason: string): MessageKey {
  return sessionFailureMessages[reason] ?? "vpn.failure.unknown";
}

export function vpnTargetFailureMessage(reason: string): MessageKey {
  return targetFailureMessages[reason] ?? "vpn.failure.unknown";
}

// 知らない語は、Go と同じく形式の誤りとして言う。
export function vpnDestinationMessage(reason: string): MessageKey {
  return Object.hasOwn(destinationMessages, reason)
    ? destinationMessages[reason as VPNDestinationReason]
    : destinationMessages.format;
}
