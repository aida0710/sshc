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
	// IKE と ESP は、装置に合わせて暗号方式を指定する。空なら、どちらも
	// L2TP装置との互換性を持たせた候補を使う。
	IKE string
	ESP string
}

// pppMTU は、PPP・L2TP・UDP・IPsec の各ヘッダを載せても 1500 に収まる大きさである。
const pppMTU = 1280

// IKEv1では、1つのproposalに複数の暗号・MAC・DH群を並べても、それぞれ先頭の
// 1つしか送られない。IKEもESPも、L2TP装置で使われる組み合わせを独立した候補として
// 提示する。
//
// ipsec.conf は指定した候補のあとに strongSwan の既定の候補も提示するが、そこから
// 送られるのは AES-128・SHA-256 と、イメージのプラグインで最初に使えるDH群
// （ECP-256）の1組だけである。MODPとSHA-1だけを受け付ける装置は、これに応答しない。
// IKEはSHA-256とMODP 3072・2048を優先し、SHA-1とMODP 2048・1024も提示する。
const defaultL2TPIKE = "aes256-sha256-modp3072,aes128-sha256-modp3072," +
	"aes256-sha256-modp2048,aes128-sha256-modp2048," +
	"aes256-sha1-modp2048,aes128-sha1-modp2048," +
	"aes256-sha1-modp1024,aes128-sha1-modp1024"

// ESPはSHA-256を優先し、HMAC-SHA1も提示する。
const defaultL2TPESP = "aes256-sha256,aes128-sha256,aes256-sha1,aes128-sha1"

// l2tpIPsecLog は、starter と charon のログを書く runtime のファイルである。agent は
// 失敗したときに、このファイルの末尾を見せる。
const l2tpIPsecLog = "ipsec.log"

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

// l2tpDocuments は、コンテナへ渡す5つの本文を返す。
func l2tpDocuments(settings L2TPSettings, secrets L2TPSecrets) map[string]string {
	if settings.IKE == "" {
		settings.IKE = defaultL2TPIKE
	}
	if settings.ESP == "" {
		settings.ESP = defaultL2TPESP
	}
	ipsec := strings.Join([]string{
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
		"    ike=" + settings.IKE,
		"    esp=" + settings.ESP,
		"",
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
		"logfile " + agentRuntimeDirectory + "/ppp.log",
		"user " + quotePPP(settings.Username),
		"password " + quotePPP(secrets.Password),
		"",
	}, "\n")
	return map[string]string{
		"strongswan.conf": strongSwanDaemonConfiguration(l2tpIPsecLog, nil),
		"ipsec.conf":      ipsec,
		// 16 進で書く。事前共有鍵に引用符や backslash があっても、strongSwan の
		// 構文として解釈されない。
		"ipsec.secrets": ": PSK " + strongSwanSecret(secrets.PreSharedKey) + "\n",
		"xl2tpd.conf": strings.Join([]string{
			"[global]",
			"port = 1701",
			"force userspace = no",
			"[lac " + connectionName + "]",
			"lns = " + serverAddressPlaceholder,
			"pppoptfile = " + agentRuntimeDirectory + "/ppp.options",
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
