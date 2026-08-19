package knownhosts

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"sshc/internal/validate"
	"strconv"
	"time"

	"golang.org/x/crypto/ssh"
)

// UnverifiedNotice は、すべてのスキャン結果に付随する。
const UnverifiedNotice = "Reaching this address proves only that something answered there. It does not prove the host's identity. Compare the fingerprint with one you obtained another way before trusting it."

// DefaultScanTimeout は、ひとつのホストを尋ねるのに掛ける上限である。
const DefaultScanTimeout = 15 * time.Second

// Candidate は、そのアドレスが提示した鍵ひとつ。ここでは Verified は常に false で
// ある。鍵が本物だと判断できるのはユーザーだけだ。
type Candidate struct {
	Host        string
	Port        int
	KeyType     string
	Key         string
	Fingerprint string
	Verified    bool
}

// Scanner はホスト鍵の候補を取得する。
//
// **外部プログラムは起こさない。** 種別ごとに握手を始め、鍵を受け取ったところで
// 断る——ssh-keyscan がしているのと同じことを、このプロセスの中で行う。
type Scanner struct {
	// Collect は、そのアドレスが提示するホスト鍵を集める。
	//
	// 関数で受け取るのは、SSH を話すパッケージがこのパッケージのパーサーを
	// 使っているからである。逆向きにも依存すると輪になる——読む場所を一つに
	// するという判断は、依存の向きを一つにするという判断でもある。
	Collect func(ctx context.Context, address string, timeout time.Duration) ([]ssh.PublicKey, error)
	Timeout time.Duration
}

// ErrNoScanner は、鍵を集める手段が配線されていないことを報告する。
var ErrNoScanner = errors.New("no host key scanner is available")

// Scan は、あるホストの鍵を尋ねる。
//
// 結果に Verified が付くことはない。アドレスに到達できたことが証明するのは、そこで
// 何かが応答したという事実だけであり、鍵を信頼する判断はユーザーのもとに残る。
func (s Scanner) Scan(ctx context.Context, host string, port int) ([]Candidate, error) {
	if err := validate.Hostname(host); err != nil {
		return nil, err
	}
	if err := validate.Port(port); err != nil {
		return nil, err
	}
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = DefaultScanTimeout
	}
	if s.Collect == nil {
		return nil, ErrNoScanner
	}
	keys, err := s.Collect(ctx, net.JoinHostPort(host, strconv.Itoa(port)), timeout)
	if err != nil {
		return nil, err
	}

	var candidates []Candidate
	for _, key := range keys {
		encoded := base64.StdEncoding.EncodeToString(key.Marshal())
		fingerprint, fingerprintErr := Fingerprint(encoded)
		if fingerprintErr != nil {
			continue
		}
		candidates = append(candidates, Candidate{
			Host:        host,
			Port:        port,
			KeyType:     key.Type(),
			Key:         encoded,
			Fingerprint: fingerprint,
		})
	}
	return candidates, nil
}
