package vpn

import (
	"encoding/json"
	"time"

	"sshc/internal/redact"
)

// agentDocument は、コンテナのagentへ標準入力で渡す設定である。
//
// 秘密を含むので、コマンド引数・環境変数・イメージ・bind mountには置かない。
// agentは読み終えたらこの文書を消す。
type agentDocument struct {
	Backend string `json:"backend"`
	// DNS は、接続先をVPNの中で名前解決するためのDNSサーバーである。
	DNS []string `json:"dns,omitempty"`
	// AttemptSeconds は、応えない相手を待つ長さ（秒）である。agent は設定を受け取った
	// ときから数え、どの待ちもその残りだけ待つ。engine が待つのをやめるより先に諦めて、
	// どこで止まったかをログへ残す。
	//
	// 時刻ではなく長さで渡す。macOS と Windows の Docker Desktop、OrbStack、colima では、
	// コンテナは Linux の VM の時計を読む。VM の時計はスリープ明けなどにホストとずれる
	// ので、ホストの時刻で渡すと、待つ長さがずれの分だけ狂う。
	AttemptSeconds int64                `json:"attemptSeconds"`
	WireGuard      *wireGuardDocument   `json:"wireguard,omitempty"`
	L2TP           *strongSwanDocument  `json:"l2tp,omitempty"`
	OpenConnect    *openConnectDocument `json:"openconnect,omitempty"`
	OpenVPN        *openVPNDocument     `json:"openvpn,omitempty"`
	IKEv2          *ikev2Document       `json:"ikev2,omitempty"`
}

// newAgentDocument は、プロファイルと秘密から、コンテナへ渡す設定を作る。
//
// now は、二段目のコードを作る時刻である。コードは 30 秒で変わるので、この文書は
// コンテナへ渡す直前に作る。
func newAgentDocument(profile Profile, secrets Secrets, now time.Time) (string, error) {
	if err := profile.Validate(); err != nil {
		return "", err
	}
	if err := profile.ValidateSecrets(secrets); err != nil {
		return "", err
	}
	document := agentDocument{
		Backend:        string(profile.Backend),
		DNS:            profile.DNS,
		AttemptSeconds: int64(agentAttempt(profile) / time.Second),
	}
	request := agentSectionRequest{profile: profile, secrets: secrets, now: now}
	if err := backends[profile.Backend].writeAgentSection(request, &document); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// redactedMark は、伏せた秘密の代わりに出す印である。
const redactedMark = "[REDACTED]"

// redactLogs は、表示する文字列から秘密を伏せる。docker logs をそのまま見せない。
//
// どの backend の秘密が混じっているかは分からないので、持っている秘密をすべて
// 伏せる。
func redactLogs(text string, secrets Secrets) string {
	var values []string
	for _, chosen := range backends {
		values = append(values, chosen.secretValues(secrets)...)
	}
	return redact.Values(text, values, redactedMark)
}
