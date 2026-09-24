import { failureCode } from "../api/client";
import type { Translate } from "../i18n/context";
import { describeVPNProblem } from "../vpn/vpnProblemMessage";

// sftpProblemText は、SFTP の操作の失敗を、画面に出す文字列にする。
//
// VPN プロファイルを付けた接続では、VPN の経路や接続先の問題で失敗することがある。その
// ときは VPN 画面と同じ言い方の1文にする。汎用の失敗の文では、VPN の設定を直せば済む
// ことが利用者に分からないからである。それ以外は problem の code を返し、code が無ければ
// 例外の文、それも無ければ fallback を返す。
export function sftpProblemText(t: Translate, error: unknown, fallback = "sftp_failed"): string {
  return describeVPNProblem(t, error) ?? (failureCode(error) || (error instanceof Error ? error.message : fallback));
}
