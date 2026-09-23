package vpn

import (
	"encoding/hex"
	"encoding/json"
	"net/netip"
	"strings"
	"time"
)

// agentDocument は、コンテナのagentへ標準入力で渡す設定である。
//
// 秘密を含むので、コマンド引数・環境変数・イメージ・bind mountには置かない。
// agentは読み終えたらこの文書を消す。
type agentDocument struct {
	Backend string           `json:"backend"`
	Target  endpointDocument `json:"target"`
	// DNS は、接続先の名前をVPNの中で引くためのDNSサーバーである。
	DNS []string `json:"dns,omitempty"`
	// AttemptSeconds は、応えない相手を待つ上限である。engine が待つのをやめる
	// より先に諦めて、どこで止まったかをログへ残す。
	AttemptSeconds int                  `json:"attemptSeconds"`
	SocketOwner    int                  `json:"socketOwner"`
	WireGuard      *wireGuardDocument   `json:"wireguard,omitempty"`
	L2TP           *l2tpDocument        `json:"l2tp,omitempty"`
	OpenConnect    *openConnectDocument `json:"openconnect,omitempty"`
}

// openConnectDocument は、agent が openconnect を呼ぶのに要るものである。
// パスワードは agent が標準入力で openconnect へ渡し、引数には置かない。
type openConnectDocument struct {
	Server   string `json:"server"`
	Username string `json:"username"`
	Protocol string `json:"protocol"`
	// ServerCertificate は、相手の証明書を固定する指紋である。空なら公的な
	// 認証局として検証させる。
	ServerCertificate string `json:"serverCertificate"`
	Password          string `json:"password"`
	// SecondFactor は、装置が二段目に聞いてきたときに送る1行である。空なら
	// 何も送らない。agent は中身を解さず、そのまま openconnect へ渡す。
	SecondFactor string `json:"secondFactor,omitempty"`
	// WaitsForApproval は、この接続が人の承認を待つかである。agent はこれを
	// ログへ書く。何も聞いてこない装置では、待っているあいだ出力が止まる。
	// 理由が書いていないと、止まったのか待っているのかが分からない。
	WaitsForApproval bool `json:"waitsForApproval,omitempty"`
}

// l2tpDocument は、agent が置くだけの本文と、agent が自分で引く相手である。
type l2tpDocument struct {
	// Server は、VPN装置の名前またはアドレスである。agent がコンテナの中で引き、
	// 設定の中の印を、引いたアドレスで置き換える。
	Server string `json:"server"`
	// Documents は、ファイル名から本文への対応である。
	Documents map[string]string `json:"documents"`
}

type endpointDocument struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type wireGuardDocument struct {
	// Configuration は wg setconf がそのまま読む本文である。
	Configuration string `json:"configuration"`
	// Address は、トンネル側でこの端末が名乗るアドレスである。wg setconf は
	// これを扱わないので、ip address add へ別に渡す。
	Address string `json:"address"`
}

// newAgentDocument は、プロファイルと秘密から、コンテナへ渡す設定を作る。
//
// now は、二段目のコードを作る時刻である。コードは 30 秒で変わるので、この文書は
// コンテナへ渡す直前に作る。
func newAgentDocument(profile Profile, secrets Secrets, socketOwner int, now time.Time) (string, error) {
	if err := profile.Validate(); err != nil {
		return "", err
	}
	if err := profile.ValidateSecrets(secrets); err != nil {
		return "", err
	}
	document := agentDocument{
		Backend:        string(profile.Backend),
		Target:         endpointDocument{Host: profile.Target.Host, Port: profile.Target.Port},
		DNS:            profile.DNS,
		AttemptSeconds: connectAttemptSeconds(profile),
		SocketOwner:    socketOwner,
	}
	switch profile.Backend {
	case WireGuard:
		document.WireGuard = &wireGuardDocument{
			Configuration: wireGuardConfiguration(profile, secrets),
			Address:       profile.WireGuard.Address,
		}
	case L2TPIPsec:
		document.L2TP = &l2tpDocument{
			Server:    profile.L2TP.Server,
			Documents: l2tpDocuments(*profile.L2TP, secrets),
		}
	case OpenConnect:
		second, err := secondFactorAnswer(*profile.OpenConnect, secrets, now)
		if err != nil {
			return "", err
		}
		document.OpenConnect = &openConnectDocument{
			Server:            profile.OpenConnect.Server,
			Username:          profile.OpenConnect.Username,
			Protocol:          openConnectProtocol(*profile.OpenConnect),
			ServerCertificate: profile.OpenConnect.ServerCertificate,
			Password:          secrets.OpenConnectPassword,
			SecondFactor:      second,
			WaitsForApproval:  waitsForApproval(profile),
		}
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// wireGuardConfiguration は、wg setconf が読む本文を作る。
//
// AllowedIPs は接続先とDNSサーバーだけにする。トンネルが運ぶのはその通信に
// 限られ、VPNの向こうのネットワーク全体を引き込まない。
func wireGuardConfiguration(profile Profile, secrets Secrets) string {
	settings := *profile.WireGuard
	lines := []string{
		"[Interface]",
		"PrivateKey = " + secrets.WireGuardPrivateKey,
		"",
		"[Peer]",
		"PublicKey = " + settings.PeerPublicKey,
		"Endpoint = " + settings.Server.Address(),
		"AllowedIPs = " + strings.Join(allowedAddresses(profile), ", "),
		// NATの内側からでも経路を保つ。相手が先に話しかけてくる構成でも、
		// こちらの経路が落ちたままにならない。
		"PersistentKeepalive = 25",
		"",
	}
	return strings.Join(lines, "\n")
}

// allowedAddresses は、トンネルが運んでよい相手である。
//
// 接続先を名前で書いた場合、そのアドレスはこの時点では分からない。まずDNS
// サーバーまでを通し、agent がVPNの中で名前を引いてから接続先を足す。
func allowedAddresses(profile Profile) []string {
	allowed := make([]string, 0, len(profile.DNS)+1)
	if _, err := netip.ParseAddr(profile.Target.Host); err == nil {
		allowed = append(allowed, profile.Target.Host+"/32")
	}
	for _, resolver := range profile.DNS {
		allowed = append(allowed, resolver+"/32")
	}
	return allowed
}

// redact は、表示する文字列から秘密を伏せる。docker logs をそのまま見せない。
//
// 16 進の表記も伏せる。IPsec の事前共有鍵は設定へ 16 進で書くので、その形のまま
// ログに現れうる。
func redact(text string, secrets Secrets) string {
	replacements := make([]string, 0, 8)
	for _, value := range []string{
		secrets.WireGuardPrivateKey, secrets.L2TPPassword, secrets.IPsecPSK,
		secrets.OpenConnectPassword, secrets.OpenConnectTOTPSecret,
	} {
		if value == "" {
			continue
		}
		replacements = append(replacements, value, "[REDACTED]")
		encoded := hex.EncodeToString([]byte(value))
		replacements = append(replacements,
			encoded, "[REDACTED]", strings.ToUpper(encoded), "[REDACTED]")
	}
	if len(replacements) == 0 {
		return text
	}
	return strings.NewReplacer(replacements...).Replace(text)
}
