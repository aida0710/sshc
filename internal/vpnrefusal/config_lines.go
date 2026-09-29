package vpnrefusal

import (
	"fmt"

	"sshc/internal/vpn"
)

// 利用者が書いた設定ファイル（OpenVPN の .ovpn、WireGuard の設定ファイル）の中の誤りの
// 言い方である。指示を名指しする理由は、「12行目の「up」は、…」の形で言う。

// directiveReasons は、指示ひとつを断った理由の言い方である。%s には断った指示が入る。
// backend に固有のものは、その backend の file の init が足す。
var directiveReasons = map[vpn.Reason]string{
	vpn.ReasonRunsCommand:   "「%s」は、コマンドやプログラムを実行する指示のため使用できません。",
	vpn.ReasonChangesRoutes: "「%s」は、経路やDNSを変更する指示のため使用できません。経路はsshcが接続先ごとに追加します。",
	vpn.ReasonDecidedBySshc: "「%s」は、sshcが決める設定のため使用できません。",
	vpn.ReasonFormat:        "「%s」の形式が正しくありません。",
}

// configFieldReasons は、どの backend の設定ファイルでも使う理由の言い方である。
var configFieldReasons = map[vpn.Reason]string{
	vpn.ReasonRunsCommand:    "コマンドやプログラムを実行する指示は使用できません。",
	vpn.ReasonChangesRoutes:  "経路やDNSを変更する指示は使用できません。",
	vpn.ReasonDecidedBySshc:  "sshcが決める設定は使用できません。",
	vpn.ReasonConfigMismatch: "設定ファイルの内容と一致しません。設定ファイルを読み込み直してください。",
}

func init() {
	for reason, sentence := range configFieldReasons {
		fieldReasons[reason] = sentence
	}
}

// configLineSentence は、設定ファイルの中の誤りを、行番号と指示を添えた文にする。
// sentence は、行を添えない場合の理由の文である。
func configLineSentence(refusal Refusal, sentence string) string {
	reason := vpn.Reason(refusal.Reason)
	if reason == vpn.ReasonMissingDirective && refusal.Directive != "" {
		if refusal.Line > 0 {
			// 行を添えて足りないと言うのは、[Peer] に PublicKey が無いときだけである。
			return fmt.Sprintf("%d行目の[Peer]に「%s」がありません。", refusal.Line, refusal.Directive)
		}
		return fmt.Sprintf("設定ファイルに「%s」がありません。", refusal.Directive)
	}
	if refusal.Line == 0 {
		return sentence
	}
	if pattern, known := directiveReasons[reason]; known && refusal.Directive != "" {
		return fmt.Sprintf("%d行目の", refusal.Line) + fmt.Sprintf(pattern, refusal.Directive)
	}
	return fmt.Sprintf("%d行目：%s", refusal.Line, sentence)
}
