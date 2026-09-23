import { ApiError } from "../api/client";
import type { MessageKey } from "../i18n/messages";

// コンテナが経路を用意できなかった理由（engine の vpn.FailureReason）と、その画面での
// 言い方。engine は vpn_session_failed の応答の reason にこの語だけを載せる。
const vpnFailureReasonMessages: Record<string, MessageKey> = {
  unknown: "vpn.failure.unknown",
  timeout: "vpn.failure.timeout",
  server_unresolved: "vpn.failure.server_unresolved",
  ipsec_negotiation: "vpn.failure.ipsec_negotiation",
  ppp_authentication: "vpn.failure.ppp_authentication",
  openconnect_failed: "vpn.failure.openconnect_failed",
  handshake_timeout: "vpn.failure.handshake_timeout",
  target_unresolved: "vpn.failure.target_unresolved",
  tunnel_lost: "vpn.failure.tunnel_lost",
};

// vpnFailureReasonMessage は、経路を用意できなかった理由の言い方を返す。
// 経路の失敗でなければ null を返す。理由の語を持たない古い engine や、この版の
// 知らない語は「理由を読み取れなかった」として扱い、ログへ案内する。
export function vpnFailureReasonMessage(error: unknown): MessageKey | null {
  if (!(error instanceof ApiError) || error.code !== "vpn_session_failed") return null;
  return vpnFailureReasonMessages[error.problem?.reason ?? ""] ?? "vpn.failure.unknown";
}
