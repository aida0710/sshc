import type { Translate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import type { VPNFieldError } from "./vpnFieldErrors";

// 設定ファイル（OpenVPN の .ovpn）の中の誤りを、何行目のどの指示かを添えた1文にする。
// 言い方は Go の internal/vpnrefusal（configLineSentence）と同じにする。

// directiveMessages は、指示ひとつを断った理由の言い方である。{directive} に指示が入る。
const directiveMessages: Partial<Record<VPNFieldError["reason"], MessageKey>> = {
  runs_command: "vpn.configLine.runs_command",
  changes_routes: "vpn.configLine.changes_routes",
  decided_by_sshc: "vpn.configLine.decided_by_sshc",
  file_reference: "vpn.configLine.file_reference",
  server_mode: "vpn.configLine.server_mode",
};

// describeConfigLine は、行を指す誤りなら、行と指示を添えた1文を返す。reason は、行を
// 添えない場合の理由の文である。行を指さない誤りなら null を返す。
export function describeConfigLine(t: Translate, error: VPNFieldError, reason: string): string | null {
  if (error.line === undefined || error.line <= 0) return null;
  const directiveMessage = directiveMessages[error.reason];
  if (directiveMessage !== undefined && error.directive !== undefined && error.directive !== "") {
    return t(directiveMessage, { line: error.line, directive: error.directive });
  }
  return t("vpn.configLine", { line: error.line, reason });
}
