import { ApiError } from "../api/client";
import type { Translate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { vpnDestinationMessage, vpnSessionFailureMessage, vpnTargetFailureMessage } from "./vpnFailureReasons";
import { describeVPNFieldRefusal, vpnFieldErrorOf } from "./vpnFieldErrors";
import { vpnRefusalMessages } from "./vpnRefusals";

// engine が VPN の操作や接続を断ったときの problem を、利用者向けの1文にする。VPN 画面、
// SFTP など、VPN の拒否を受け取るどの画面も同じ言い方で見せる。組み立て方は Go の
// vpnrefusal.Sentence と同じにする。

const refusalMessages: Record<string, MessageKey> = vpnRefusalMessages;

// describeReasonedRefusal は、理由の語で言い方が決まる拒否を1文にする。そうでなければ null。
function describeReasonedRefusal(t: Translate, code: string, reason: string): string | null {
  switch (code) {
    case "vpn_session_failed":
      return t("vpn.sessionFailed", { reason: t(vpnSessionFailureMessage(reason)) });
    case "vpn_target_failed":
      return t("vpn.targetFailed", { reason: t(vpnTargetFailureMessage(reason)) });
    case "vpn_destination_invalid":
      return t(vpnDestinationMessage(reason));
    default:
      return null;
  }
}

// describeVPNProblem は、VPN の拒否なら1文を返す。VPN の拒否でなければ null を返す。
export function describeVPNProblem(t: Translate, error: unknown): string | null {
  if (!(error instanceof ApiError)) return null;
  const reasoned = describeReasonedRefusal(t, error.code, error.problem?.reason ?? "");
  if (reasoned !== null) return reasoned;
  const key = refusalMessages[error.code];
  if (key === undefined) return null;
  // 項目を名指しする拒否は「項目: 理由」で言う。項目が分からなければ、code の文にする。
  const field = vpnFieldErrorOf(error);
  return (field === null ? null : describeVPNFieldRefusal(t, field)) ?? t(key);
}
