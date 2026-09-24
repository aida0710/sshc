package vpn

import (
	"encoding/json"
	"strings"
	"time"
)

// agentDocument は、コンテナのagentへ標準入力で渡す設定である。
//
// 秘密を含むので、コマンド引数・環境変数・イメージ・bind mountには置かない。
// agentは読み終えたらこの文書を消す。
type agentDocument struct {
	Backend string `json:"backend"`
	// DNS は、接続先をVPNの中で名前解決するためのDNSサーバーである。
	DNS []string `json:"dns,omitempty"`
	// Deadline は、応えない相手を待つのをやめる時刻（UNIX 秒）である。agent の
	// 待ちはどれも、この時刻までの残りだけ待つ。engine が待つのをやめるより
	// 先に諦めて、どこで止まったかをログへ残す。
	//
	// engine とコンテナは同じカーネルの時計を見るので、時刻で渡してよい。
	Deadline    int64                `json:"deadline"`
	WireGuard   *wireGuardDocument   `json:"wireguard,omitempty"`
	L2TP        *l2tpDocument        `json:"l2tp,omitempty"`
	OpenConnect *openConnectDocument `json:"openconnect,omitempty"`
}

// newAgentDocument は、プロファイルと秘密から、コンテナへ渡す設定を作る。
//
// now は、二段目のコードを作る時刻であり、締め切りを数え始める時刻である。
// コードは 30 秒で変わるので、この文書はコンテナへ渡す直前に作る。
func newAgentDocument(profile Profile, secrets Secrets, now time.Time) (string, error) {
	if err := profile.Validate(); err != nil {
		return "", err
	}
	if err := profile.ValidateSecrets(secrets); err != nil {
		return "", err
	}
	document := agentDocument{
		Backend:  string(profile.Backend),
		DNS:      profile.DNS,
		Deadline: agentDeadline(profile, now).Unix(),
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

// redact は、表示する文字列から秘密を伏せる。docker logs をそのまま見せない。
//
// どの backend の秘密が混じっているかは分からないので、持っている秘密をすべて
// 伏せる。
func redact(text string, secrets Secrets) string {
	var replacements []string
	for _, chosen := range backends {
		for _, value := range chosen.secretValues(secrets) {
			if value != "" {
				replacements = append(replacements, value, redactedMark)
			}
		}
	}
	if len(replacements) == 0 {
		return text
	}
	return strings.NewReplacer(replacements...).Replace(text)
}
