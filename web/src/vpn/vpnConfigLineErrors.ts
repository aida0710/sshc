import type { Translate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import type { VPNFieldError } from "./vpnFieldErrors";

// 設定ファイル（OpenVPN の .ovpn、WireGuard の設定ファイル）の中の誤りを、何行目のどの指示かを
// 添えた1文にする。言い方は Go の internal/vpnrefusal（configLineSentence）と同じにする。

// directiveMessages は、指示ひとつを断った理由の言い方である。{directive} に指示が入る。
const directiveMessages: Partial<Record<VPNFieldError["reason"], MessageKey>> = {
  runs_command: "vpn.configLine.runs_command",
  changes_routes: "vpn.configLine.changes_routes",
  decided_by_sshc: "vpn.configLine.decided_by_sshc",
  format: "vpn.configLine.format",
  file_reference: "vpn.configLine.file_reference",
  server_mode: "vpn.configLine.server_mode",
  not_ipv4: "vpn.configLine.not_ipv4",
  unroutable: "vpn.configLine.unroutable",
  out_of_range: "vpn.configLine.out_of_range",
  duplicate: "vpn.configLine.duplicate",
  misplaced_directive: "vpn.configLine.misplaced_directive",
  not_in_allowed_ips: "vpn.configLine.not_in_allowed_ips",
};

// describeConfigLine は、行か項目を指す誤りなら、行と項目を添えた1文を返す。reason は、行を
// 添えない場合の理由の文である。行も項目も指さない誤りなら null を返す。
export function describeConfigLine(t: Translate, error: VPNFieldError, reason: string): string | null {
  const line = error.line ?? 0;
  const directive = error.directive ?? "";
  if (error.reason === "missing_directive" && directive !== "") {
    // 行を添えて足りないと言うのは、[Peer] に PublicKey が無いときだけである。
    return line > 0
      ? t("vpn.configLine.missing_directive", { line, directive })
      : t("vpn.configFile.missing_directive", { directive });
  }
  if (line <= 0) return null;
  const directiveMessage = directiveMessages[error.reason];
  if (directiveMessage !== undefined && directive !== "") {
    return t(directiveMessage, { line, directive });
  }
  return t("vpn.configLine", { line, reason });
}
