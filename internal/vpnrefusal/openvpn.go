package vpnrefusal

import "sshc/internal/vpn"

// OpenVPN の設定ファイル（.ovpn）とプロファイルを受け取れなかった理由の言い方である。
// 指示を名指しする理由は、「12行目の「ca」は、…」の形で言う（config_lines.go）。

// openVPNDirectiveReasons は、OpenVPN の指示ひとつを断った理由の言い方である。
var openVPNDirectiveReasons = map[vpn.Reason]string{
	vpn.ReasonFileReference: "「%[1]s」はファイルを指定しています。コンテナからはファイルを読めないため、" +
		"中身を<%[1]s>〜</%[1]s>の形で設定ファイルに埋め込んでください。",
	vpn.ReasonServerMode: "「%s」は、VPNサーバー用の設定のため使用できません。",
}

// openVPNFieldReasons は、OpenVPN の設定ファイルとプロファイルに固有の理由の言い方である。
var openVPNFieldReasons = map[vpn.Reason]string{
	vpn.ReasonFileReference:     "ファイルを指定する指示は使用できません。中身を設定ファイルに埋め込んでください。",
	vpn.ReasonServerMode:        "VPNサーバー用の設定は使用できません。",
	vpn.ReasonUnsupportedInline: "埋め込みのブロックに対応していない指示です。",
	vpn.ReasonUnclosedInline:    "埋め込みのブロックの終わりの行がありません。",
	vpn.ReasonNotClient: "OpenVPNのクライアント用の設定ファイルではありません。" +
		"clientまたはtls-clientを含む設定ファイルを指定してください。",
	vpn.ReasonNoRemote:         "接続するVPNサーバー（remote）が設定ファイルにありません。",
	vpn.ReasonRequiredByConfig: "設定ファイルがユーザー名とパスワードを要求しています（auth-user-pass）。ユーザー名を入力してください。",
}

// openVPNSessionReasons は、OpenVPN の経路を用意できなかった理由の言い方である。
var openVPNSessionReasons = map[vpn.FailureReason]string{
	vpn.FailureOpenVPNAuthentication: "VPNサーバーが認証を拒否しました。ユーザー名とパスワードを確認してください。",
	vpn.FailureOpenVPNTLS: "TLSのハンドシェイクに失敗しました。設定ファイルの証明書と鍵、" +
		"サーバーの証明書の確認の設定（remote-cert-tlsなど）を確認してください。",
	vpn.FailureOpenVPNNoResponse: "VPNサーバーから応答がありません。設定ファイルのremote、ネットワーク、" +
		"tls-authとtls-cryptの鍵を確認してください。",
	vpn.FailureOpenVPNConfiguration: "OpenVPNが設定ファイルを読み込めませんでした。ログで理由を確認してください。",
	vpn.FailureOpenVPN:              "OpenVPNが終了しました。ログで理由を確認してください。",
}

// init は、OpenVPN に固有の理由を、共通の言い方の表に加える。文は Sentence と
// fieldSentence がまとめて組み立てる。
func init() {
	for reason, sentence := range openVPNSessionReasons {
		sessionReasons[reason] = sentence
	}
	for reason, sentence := range openVPNFieldReasons {
		fieldReasons[reason] = sentence
	}
	for reason, pattern := range openVPNDirectiveReasons {
		directiveReasons[reason] = pattern
	}
}
