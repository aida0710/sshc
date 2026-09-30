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

// openVPNRouteReasons は、OpenVPN の経路を用意できなかった理由の言い方である。
var openVPNRouteReasons = map[vpn.FailureReason]string{
	vpn.FailureOpenVPNAuthentication: "VPNサーバーが認証を拒否しました。ユーザー名とパスワードを確認してください。",
	vpn.FailureOpenVPNTLS: "TLSのハンドシェイクに失敗しました。設定ファイルの証明書と鍵、" +
		"サーバーの証明書の確認の設定（remote-cert-tlsなど）を確認してください。",
	vpn.FailureOpenVPNNoResponse: "VPNサーバーから応答がありません。設定ファイルのremote、ネットワーク、" +
		"tls-authとtls-cryptの鍵を確認してください。",
	vpn.FailureOpenVPNConfiguration: "OpenVPNが設定ファイルを読み込めませんでした。ログで理由を確認してください。",
	vpn.FailureOpenVPN:              "OpenVPNが終了しました。ログで理由を確認してください。",
}

// openVPNEnglishDirectiveReasons は、openVPNDirectiveReasons の英語である。
var openVPNEnglishDirectiveReasons = map[vpn.Reason]string{
	vpn.ReasonFileReference: "\"%[1]s\" names a file, which the container cannot read. " +
		"Embed its contents as <%[1]s>…</%[1]s>.",
	vpn.ReasonServerMode: "\"%s\" is a VPN server setting, so it cannot be used.",
}

// openVPNEnglishFieldReasons は、openVPNFieldReasons の英語である。
var openVPNEnglishFieldReasons = map[vpn.Reason]string{
	vpn.ReasonFileReference:     "Directives that name a file cannot be used. Embed the contents in the configuration file.",
	vpn.ReasonServerMode:        "VPN server settings cannot be used.",
	vpn.ReasonUnsupportedInline: "This directive cannot be embedded as a block.",
	vpn.ReasonUnclosedInline:    "An embedded block has no closing line.",
	vpn.ReasonNotClient: "This is not an OpenVPN client configuration file. " +
		"Use a file that has client or tls-client.",
	vpn.ReasonNoRemote: "The configuration file names no VPN server (remote).",
	vpn.ReasonRequiredByConfig: "The configuration file asks for a username and a password (auth-user-pass). " +
		"Enter the username.",
}

// openVPNEnglishRouteReasons は、openVPNRouteReasons の英語である。
var openVPNEnglishRouteReasons = map[vpn.FailureReason]string{
	vpn.FailureOpenVPNAuthentication: "The VPN server refused the authentication. Check the username and the password.",
	vpn.FailureOpenVPNTLS: "The TLS handshake failed. Check the certificates and keys in the file, " +
		"and how it verifies the server (remote-cert-tls and similar).",
	vpn.FailureOpenVPNNoResponse: "The VPN server did not answer. Check remote in the file, the network, " +
		"and the tls-auth and tls-crypt keys.",
	vpn.FailureOpenVPNConfiguration: "OpenVPN could not load the configuration file. Check the logs for the reason.",
	vpn.FailureOpenVPN:              "OpenVPN exited. Check the logs for the reason.",
}

// init は、OpenVPN に固有の理由を、共通の言い方の表に加える。文は Sentence と
// fieldSentence がまとめて組み立てる。
func init() {
	for reason, sentence := range openVPNRouteReasons {
		routeReasons[reason] = sentence
	}
	for reason, sentence := range openVPNFieldReasons {
		fieldReasons[reason] = sentence
	}
	for reason, pattern := range openVPNDirectiveReasons {
		directiveReasons[reason] = pattern
	}
	for reason, sentence := range openVPNEnglishRouteReasons {
		englishRouteReasons[reason] = sentence
	}
	for reason, sentence := range openVPNEnglishFieldReasons {
		englishFieldReasons[reason] = sentence
	}
	for reason, pattern := range openVPNEnglishDirectiveReasons {
		englishDirectiveReasons[reason] = pattern
	}
}
