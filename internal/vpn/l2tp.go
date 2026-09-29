package vpn

import (
	"strconv"
	"strings"
)

// l2tp_ipsec backend。strongSwan、xl2tpd、pppd がそれぞれ読む本文を組み立てる。
//
// コンテナの agent は受け取った本文をそのまま置き、相手のアドレスだけを自分で
// 引いて埋める。IPsec は相手のアドレスを設定に書くので、名前を引く場所が違えば
// 別の装置へ繋ぎうる。

// L2TPSettings は、l2tp_ipsec backendの秘密でない設定である。
type L2TPSettings struct {
	// Server は、VPN装置の名前またはアドレスである。名前はコンテナの中で
	// 引く。IPsecは相手のアドレスを設定に書くので、引く場所が違えば別の装置へ
	// 繋ぎうる。
	Server string
	// Username は、VPNの利用者名である。パスワードはVaultにある。
	Username string
	// IKE と ESP は、装置に合わせて暗号方式を指定する。空ならIKEは
	// strongSwanの既定、ESPはL2TP向けの互換性を持たせた候補を使う。
	IKE string
	ESP string
}

// pppMTU は、PPP・L2TP・UDP・IPsec の各ヘッダを載せても 1500 に収まる大きさである。
const pppMTU = 1280

// IKEv1では1つのproposalに複数の暗号・MACを並べても先頭しか送られない。
// SHA-256を優先し、L2TP装置で使われるHMAC-SHA1も独立した候補として提示する。
const defaultL2TPESP = "aes256-sha256,aes128-sha256,aes256-sha1,aes128-sha1"

type l2tpBackend struct{}

func (l2tpBackend) device() string { return "/dev/ppp" }

// capabilities は既定のままにする。strongSwan・xl2tpd・pppd がどの権限を使うかを、
// 実際のVPN装置に対して確かめられていない。
func (l2tpBackend) capabilities() []string { return commonCapabilities }

func (l2tpBackend) validateSettings(profile Profile) error {
	settings := profile.L2TP
	if settings == nil {
		return fieldError(ErrSettings, "l2tp", ReasonRequired)
	}
	if err := validateServerName("l2tp.server", settings.Server); err != nil {
		return err
	}
	if err := validateUsername("l2tp.username", settings.Username); err != nil {
		return err
	}
	return validateProposals("l2tp", settings.IKE, settings.ESP)
}

func (l2tpBackend) validateSecrets(_ Profile, secrets Secrets) error {
	var stored L2TPSecrets
	if secrets.L2TP != nil {
		stored = *secrets.L2TP
	}
	if err := requireSecret("secrets."+SecretKeyL2TPPassword, stored.Password, maxSecretLength); err != nil {
		return err
	}
	return requireSecret("secrets."+SecretKeyIPsecPSK, stored.PreSharedKey, maxSecretLength)
}

func (l2tpBackend) writeAgentSection(request agentSectionRequest, document *agentDocument) error {
	document.L2TP = &strongSwanDocument{
		Server:    request.profile.L2TP.Server,
		Documents: l2tpDocuments(*request.profile.L2TP, *request.secrets.L2TP),
	}
	return nil
}

// secretValues は、パスワードと事前共有鍵、その16進表記を返す。事前共有鍵は
// 設定へ16進で書くので、その形のままログに現れうる。
func (l2tpBackend) secretValues(secrets Secrets) []string {
	if secrets.L2TP == nil {
		return nil
	}
	return append([]string{secrets.L2TP.Password}, secretAndHexForms(secrets.L2TP.PreSharedKey)...)
}

func (l2tpBackend) waitsForApproval(Profile) bool { return false }

func (l2tpBackend) ownSecrets(_ Profile, secrets Secrets) Secrets { return Secrets{L2TP: secrets.L2TP} }

// l2tpDocuments は、コンテナへ渡す4つの本文を返す。
func l2tpDocuments(settings L2TPSettings, secrets L2TPSecrets) map[string]string {
	if settings.ESP == "" {
		settings.ESP = defaultL2TPESP
	}
	proposals := ""
	for _, proposal := range []struct{ field, value string }{{"ike", settings.IKE}, {"esp", settings.ESP}} {
		if proposal.value != "" {
			proposals += "    " + proposal.field + "=" + proposal.value + "\n"
		}
	}
	ipsec := strings.Join([]string{
		"config setup",
		`    charondebug="ike 1, knl 1, cfg 0"`,
		"conn " + connectionName,
		"    keyexchange=ikev1",
		"    authby=psk",
		// transport mode。L2TP は IPsec の中を通る UDP 1701 であり、
		// トンネルモードの二重化は要らない。
		"    type=transport",
		"    left=%defaultroute",
		"    leftprotoport=17/1701",
		"    right=" + serverAddressPlaceholder,
		"    rightid=%any",
		"    rightprotoport=17/1701",
		// NAT の内側から繋ぐことが多い。UDP 4500 へ包む。
		"    forceencaps=yes",
		"    keyingtries=1",
		"    dpdaction=clear",
		"    dpddelay=30s",
		"    auto=add",
		proposals,
	}, "\n")
	ppp := strings.Join([]string{
		"ipcp-accept-local",
		"ipcp-accept-remote",
		// 相手にこちらを認証させない。こちらが相手を認証するのは IPsec の仕事である。
		"noauth",
		"noccp",
		"noipdefault",
		// 既定経路を奪わせない。接続先への /32 だけを、この名前空間に作る。
		"nodefaultroute",
		"noipv6",
		"unit 0",
		"mtu " + strconv.Itoa(pppMTU),
		"mru " + strconv.Itoa(pppMTU),
		"lcp-echo-interval 30",
		"lcp-echo-failure 3",
		"hide-password",
		"logfile /run/sshc-vpn/ppp.log",
		"user " + quotePPP(settings.Username),
		"password " + quotePPP(secrets.Password),
		"",
	}, "\n")
	return map[string]string{
		"ipsec.conf": ipsec,
		// 16 進で書く。事前共有鍵に引用符や backslash があっても、strongSwan の
		// 構文として解釈されない。
		"ipsec.secrets": ": PSK " + strongSwanSecret(secrets.PreSharedKey) + "\n",
		"xl2tpd.conf": strings.Join([]string{
			"[global]",
			"port = 1701",
			"force userspace = no",
			"[lac " + connectionName + "]",
			"lns = " + serverAddressPlaceholder,
			"pppoptfile = /run/sshc-vpn/ppp.options",
			"ppp debug = no",
			"autodial = no",
			"redial = no",
			"length bit = yes",
			"",
		}, "\n"),
		"ppp.options": ppp,
	}
}

// quotePPP は、pppd の options ファイルの1語として書ける形にする。
func quotePPP(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`
}
