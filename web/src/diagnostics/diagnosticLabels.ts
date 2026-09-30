import type { Translate } from "../i18n/context";
import { codeText, type CodeMessages } from "../i18n/codeText";
import type { MessageKey } from "../i18n/messages";

// engine の検査の結果は機械向けの値なので、画面では表を引いて文にする。
// Diagnostics と接続エディタの確認が同じ言い方を使う。

// 疎通の結果（internal/diagnostics/reachability.go の Reachability*）。
const reachabilityOutcomes: CodeMessages = {
  reached: "diag.outcome.reached",
  refused: "diag.outcome.refused",
  timeout: "diag.outcome.timeout",
  dns_failure: "diag.outcome.dnsFailure",
  failed: "diag.outcome.failed",
  not_checked: "diag.outcome.notChecked",
};

// 認証テストの結果（internal/diagnostics/authentication.go の Outcome*）。
const authenticationOutcomes: CodeMessages = {
  authenticated: "diag.outcome.authenticated",
  authentication_denied: "diag.outcome.authenticationDenied",
  host_key_unknown: "diag.outcome.hostKeyUnknown",
  host_key_changed: "diag.outcome.hostKeyChanged",
  dns_failure: "diag.outcome.dnsFailure",
  connection_refused: "diag.outcome.refused",
  timeout: "diag.outcome.timeout",
  failed: "diag.outcome.authenticationFailed",
};

// 設定を単純な継承として示せない理由（internal/effective の Complexity*）。
const complexityNotes: CodeMessages = {
  wildcard_pattern: "diag.complexity.wildcardPattern",
  negated_pattern: "diag.complexity.negatedPattern",
  match_block: "diag.complexity.matchBlock",
  duplicate_alias: "diag.complexity.duplicateAlias",
  proxy_ignored: "diag.complexity.proxyIgnored",
  unresolved_include: "diag.complexity.unresolvedInclude",
  jump_invalid: "diag.complexity.jumpInvalid",
  jump_cycle: "diag.complexity.jumpCycle",
  jump_depth_exceeded: "diag.complexity.jumpDepthExceeded",
  jump_unresolved: "diag.complexity.jumpUnresolved",
};

export function describeReachability(t: Translate, outcome: string): string {
  return codeText(t, outcome, { messages: reachabilityOutcomes, fallback: "diag.outcome.failed" });
}

// reachabilityNoticeKey は、疎通の結果に添える注意の文を選ぶ。VPN プロファイルを付けた
// 接続は接続先へ直接つながないので（not_checked）、踏み台を使わない直接の接続の注意では
// なく、VPN を経由しないので確かめていないことと、認証の確認が VPN を経由することを言う。
export function reachabilityNoticeKey(outcome: string): MessageKey {
  return outcome === "not_checked" ? "diag.vpnRouteNotice" : "diag.directDialNotice";
}

export function describeAuthentication(t: Translate, outcome: string): string {
  return codeText(t, outcome, { messages: authenticationOutcomes, fallback: "diag.outcome.authenticationFailed" });
}

export function describeComplexity(t: Translate, code: string): string {
  return codeText(t, code, { messages: complexityNotes, fallback: "diag.complexity.other" });
}

// complexityShowsDetail は、engine の detail を補足として出すかを返す。detail はたいてい
// 英語の説明文で、画面の文と重なる。値そのもの（読めなかった ProxyJump の値）だけを出す。
export function complexityShowsDetail(code: string): boolean {
  return code === "jump_invalid";
}
