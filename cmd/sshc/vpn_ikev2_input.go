package main

import (
	"sshc/internal/application"
	"sshc/internal/vpn"
)

// IKEv2/IPsec のプロファイルの入力（sshc vpn add と sshc vpn edit）。

// readIKEv2Profile は、IKEv2/IPsec の設定と、認証の方式に合うシークレットを読む。
func readIKEv2Profile(
	p vpnProfilePrompter, current *application.IKEv2Profile, keeps bool,
) (*application.IKEv2Profile, []vpnSecretField, error) {
	previous := application.IKEv2Profile{Authentication: vpn.IKEv2AuthenticationEAP}
	if current != nil {
		previous = *current
	}
	server, err := p.required("VPN server (host)", previous.Server)
	if err != nil {
		return nil, nil, err
	}
	authentication, err := p.required("Authentication (eap-mschapv2/psk)", previous.Authentication)
	if err != nil {
		return nil, nil, err
	}
	psk := authentication == vpn.IKEv2AuthenticationPSK
	identityLabel := "VPN username"
	if psk {
		identityLabel = "Local ID"
	}
	identity, err := p.required(identityLabel, previous.Identity)
	if err != nil {
		return nil, nil, err
	}
	serverIdentity, err := p.optional("Server ID (blank uses the server name)", previous.ServerIdentity)
	if err != nil {
		return nil, nil, err
	}
	// 事前共有鍵では、サーバーも事前共有鍵で認証するので、証明書を尋ねない。
	var certificate string
	if !psk {
		if certificate, err = readCACertificate(p, previous.CACertificate); err != nil {
			return nil, nil, err
		}
	}
	// 暗号スイートは、サーバーと合わないときだけ書く。空なら strongSwan の既定に任せる。
	ike, err := p.optional("IKE proposals", previous.IKE)
	if err != nil {
		return nil, nil, err
	}
	esp, err := p.optional("ESP proposals", previous.ESP)
	if err != nil {
		return nil, nil, err
	}
	// 認証の方式を変えたら、保存済みのシークレットは別の方式のものなので使えない。
	keepsSecret := keeps && previous.Authentication == authentication
	field := vpnSecretField{name: vpn.SecretKeyIKEv2Password}
	label := "VPN password"
	if psk {
		field.name, label = vpn.SecretKeyIKEv2PSK, "IKE pre-shared key"
	}
	if field.value, err = p.secret(label, keepsSecret); err != nil {
		return nil, nil, err
	}
	secrets := sentSecrets(field)
	if server == "" || authentication == "" || identity == "" {
		return nil, secrets, errVPNInputMissing
	}
	if err := requireSecret(field.value, keepsSecret); err != nil {
		return nil, secrets, err
	}
	return &application.IKEv2Profile{
		Server: server, Authentication: authentication, Identity: identity, ServerIdentity: serverIdentity,
		CACertificate: certificate, IKE: ike, ESP: esp,
	}, secrets, nil
}

// readCACertificate は、サーバーの証明書を確かめる CA の証明書を、PEM のファイルから
// 読む。PEM は複数行なので、ターミナルへ貼り付けさせずにファイルの場所を尋ねる。
//
// 空欄は、作成では公的な認証局で確かめること、編集では保存済みの証明書のままを表す。
// clearWord は、保存済みの証明書を消して公的な認証局で確かめる。
func readCACertificate(p vpnProfilePrompter, current string) (string, error) {
	label := "CA certificate file (PEM, blank to trust public CAs): "
	if current != "" {
		label = "CA certificate file (blank keeps the saved certificate, " + clearWord + " to trust public CAs): "
	}
	path, err := promptVisibleSetup(p.ctx, p.stdin, p.prompt, label, "")
	if err != nil {
		return "", err
	}
	switch path {
	case "":
		return current, nil
	case clearWord:
		return "", nil
	}
	// 送っても断られる大きさのファイルは、engine と同じ文で送る前に断る。形は engine が
	// 確かめ、誤りは項目の誤りとして返す。
	certificate, err := readVPNInputFile(vpnInputFile{
		path: path, limit: vpn.MaxCACertificateLength, description: "CA certificate file",
		kind: vpn.ErrSettings, field: vpn.IKEv2CACertificateField,
	})
	if err != nil {
		return "", err
	}
	return string(certificate), nil
}
