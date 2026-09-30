package vpnrefusal

import "sshc/internal/vpn"

// 利用者が書いた設定ファイル（OpenVPN の .ovpn、WireGuard の設定ファイル）の中の誤りの
// 言い方である。指示を名指しする理由は、「12行目の「up」は、…」（英語では
// 「Line 12: "up" …」）の形で言う（phrasebook.configLineSentence）。

// directiveReasons は、指示ひとつを断った理由の言い方である。%s には断った指示が入る。
// backend に固有のものは、その backend の file の init が足す。
var directiveReasons = map[vpn.Reason]string{
	vpn.ReasonRunsCommand:   "「%s」は、コマンドやプログラムを実行する指示のため使用できません。",
	vpn.ReasonChangesRoutes: "「%s」は、ルートやDNSを変更する指示のため使用できません。ルートはsshcが接続先ごとに追加します。",
	vpn.ReasonDecidedBySshc: "「%s」は、sshcが決める設定のため使用できません。",
	vpn.ReasonFormat:        "「%s」の形式が正しくありません。",
}

// configFieldReasons は、どの backend の設定ファイルでも使う理由の言い方である。
var configFieldReasons = map[vpn.Reason]string{
	vpn.ReasonRunsCommand:    "コマンドやプログラムを実行する指示は使用できません。",
	vpn.ReasonChangesRoutes:  "ルートやDNSを変更する指示は使用できません。",
	vpn.ReasonDecidedBySshc:  "sshcが決める設定は使用できません。",
	vpn.ReasonConfigMismatch: "設定ファイルの内容と一致しません。設定ファイルを読み込み直してください。",
}

// englishDirectiveReasons は、directiveReasons の英語である。
var englishDirectiveReasons = map[vpn.Reason]string{
	vpn.ReasonRunsCommand:   "\"%s\" runs a command or loads a program, so it cannot be used.",
	vpn.ReasonChangesRoutes: "\"%s\" changes routes or DNS, so it cannot be used. sshc adds a route for each destination.",
	vpn.ReasonDecidedBySshc: "\"%s\" is decided by sshc, so it cannot be used.",
	vpn.ReasonFormat:        "\"%s\" is not written in a form it accepts.",
}

// englishConfigFieldReasons は、configFieldReasons の英語である。
var englishConfigFieldReasons = map[vpn.Reason]string{
	vpn.ReasonRunsCommand:    "Directives that run a command or load a program cannot be used.",
	vpn.ReasonChangesRoutes:  "Directives that change routes or DNS cannot be used.",
	vpn.ReasonDecidedBySshc:  "Settings that sshc decides cannot be used.",
	vpn.ReasonConfigMismatch: "This does not match the configuration file. Load the configuration file again.",
}

func init() {
	for reason, sentence := range configFieldReasons {
		fieldReasons[reason] = sentence
	}
	for reason, sentence := range englishConfigFieldReasons {
		englishFieldReasons[reason] = sentence
	}
}
