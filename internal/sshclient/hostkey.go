package sshclient

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"

	"sshc/internal/knownhosts"
)

// ホスト鍵を受け入れなかった理由。
var (
	// ErrHostKeyChanged は、known_hosts にある鍵と違う鍵を提示したホストを断る。
	//
	// ここだけはユーザーに判断させない。known_hosts にあって鍵が違うのは中間者攻撃の
	// 形そのものであり、「続けますか」と尋ねること自体が攻撃の成立条件になる。
	ErrHostKeyChanged = errors.New("the host key does not match the one in known_hosts")
	// ErrHostKeyUnknown は、未知のホストを受け入れなかったことを報告する。
	ErrHostKeyUnknown = errors.New("this host is not in known_hosts")
	// ErrHostKeyRevoked は、@revoked と印の付いた鍵を断る。
	ErrHostKeyRevoked = errors.New("this host key is marked revoked in known_hosts")
)

// HostKeys は、known_hosts との突き合わせである。
//
// 読み書きを関数として受け取り、UI と接続処理が同じ known_hosts を使用できるようにする。
type HostKeys struct {
	// Read は known_hosts の中身を返す。nil なら、既知のホストは一つも無い。
	Read func() ([]byte, error)
	// Add は受け入れた鍵を書く。nil なら覚えない。接続はできるが、次も尋ねる。
	Add func(candidate knownhosts.Candidate) error
}

// Callback は、この接続のためのホスト鍵検証を返す。
//
// 問いを出す先を引数で受け取るのは、それが接続ごとに違うからである。尋ねる
// のは、その接続を開いた端末でなければならない。別の端末に出た問いは、
// 誰も判定できないまま接続を止める。
func (h HostKeys) Callback(target Target, prompt Prompter) ssh.HostKeyCallback {
	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		return h.verify(target, key, prompt)
	}
}

// defaultHostKeyAlgorithms は、他に手がかりが無いときに名乗る順である。
//
// 順番は OpenSSH の既定に合わせてある。x/crypto の既定表は ECDSA と RSA を
// Ed25519 より前に置くが、それに従うと、初めて繋ぐホストについて覚える鍵の種類が
// `ssh` の覚えるものと変わる。同じ known_hosts を二つのクライアントが書くのだから、
// 順番は揃っている方がよい。
//
// 証明書のアルゴリズムは入れない。このクライアントは証明書を読まないので、
// 名乗れば、受け取っても突き合わせられないものを相手に選ばせることになる。
//
// 末尾の ssh-rsa は SHA-1 の署名であり、OpenSSH の既定からはもう外れている。
// ここに残すのは、それしか持たない古いサーバーへ今日繋がっているからで、
// 最後にあるので他に選べるものがあれば選ばれない。
var defaultHostKeyAlgorithms = []string{
	ssh.KeyAlgoED25519,
	ssh.KeyAlgoECDSA256, ssh.KeyAlgoECDSA384, ssh.KeyAlgoECDSA521,
	ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256,
	ssh.KeyAlgoRSA,
}

// Algorithms は、この接続で名乗るホスト鍵アルゴリズムを優先順に返す。
//
// known_hosts に持っている種類を先に置く。これが無いと順番を決めるのは
// x/crypto の既定表になり、そこでは RSA と ECDSA が Ed25519 より前にある。
// 三種類の鍵を持つ普通の Ubuntu が相手だと RSA が選ばれ、known_hosts にある
// のが ed25519 の 1 行だけなら、正しいホストの正しい鍵が「一致しない鍵」として
// 現れる。実際そうなっていた。変わったのは相手ではなく、こちらの選び方で
// ある。OpenSSH が同じ場面で ed25519 を選ぶのは、すでに持っている種類を
// 先に置いているからで、ここがしているのはそれと同じことである。
//
// 設定に HostKeyAlgorithms が書かれていれば、それが順序である。OpenSSH は
// その指定があるとき known_hosts による並べ替えを行わない。ユーザーが決めた順を、
// こちらの都合で作り変えない。
//
// 知らないホストでは既定の順を返す。持っていない鍵について主張することは無いが、
// 何も渡さなければ x/crypto の順になり、それは `ssh` の順ではない。
func (h HostKeys) Algorithms(target Target) []string {
	if len(target.HostKeyAlgorithms) > 0 {
		return target.HostKeyAlgorithms
	}
	if h.Read == nil {
		return defaultHostKeyAlgorithms
	}
	contents, err := h.Read()
	if err != nil {
		// 読めないことをここで報告する必要はない。同じ読み取りは検証でもう一度
		// 起き、そこが接続を止める。
		return defaultHostKeyAlgorithms
	}
	field := hostField(target.HostName, target.Port)
	var algorithms []string
	seen := map[string]bool{}
	for _, line := range knownhosts.ParseFile(contents).Lines {
		entry := line.Entry
		// 印の付いた行は、この接続で受け入れる鍵ではない。@revoked は拒む鍵で
		// あり、@cert-authority は鍵ではなく署名者である。
		if entry == nil || entry.Marker != "" || !entry.MatchesHost(field) {
			continue
		}
		for _, algorithm := range signatureAlgorithms(entry.KeyType) {
			if seen[algorithm] {
				continue
			}
			seen[algorithm] = true
			algorithms = append(algorithms, algorithm)
		}
	}
	if len(algorithms) == 0 {
		return defaultHostKeyAlgorithms
	}
	return algorithms
}

// signatureAlgorithms は、known_hosts が書く鍵の種類を、交渉で名乗る署名
// アルゴリズムへ広げる。
//
// RSA だけは、鍵の種類と署名アルゴリズムが一対一ではない。known_hosts は
// ssh-rsa としか書かないが、それをそのまま名乗ると SHA-1 の署名だけを求める
// ことになり、SHA-1 を断る今どきのサーバーとは繋がらない。同じ鍵で名乗れる
// 三つを OpenSSH と同じ順で返す。
func signatureAlgorithms(keyType string) []string {
	if keyType == ssh.KeyAlgoRSA {
		return []string{ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256, ssh.KeyAlgoRSA}
	}
	return []string{keyType}
}

func (h HostKeys) verify(target Target, key ssh.PublicKey, prompt Prompter) error {
	field := hostField(target.HostName, target.Port)
	offered := base64.StdEncoding.EncodeToString(key.Marshal())

	matchedHost := false
	if h.Read != nil {
		contents, err := h.Read()
		if err != nil {
			return err
		}
		for _, line := range knownhosts.ParseFile(contents).Lines {
			entry := line.Entry
			if entry == nil || !entry.MatchesHost(field) {
				continue
			}
			if strings.EqualFold(entry.Marker, "@revoked") {
				if entry.Key == offered {
					return ErrHostKeyRevoked
				}
				continue
			}
			// @cert-authority は、その鍵で署名された証明書を認めるという意味であり、
			// ホスト鍵そのものではない。証明書はこのクライアントがまだ扱わないので、
			// 一致の判断からは外す。
			if entry.Marker != "" {
				continue
			}
			matchedHost = true
			if entry.Key == offered {
				return nil
			}
		}
	}
	if matchedHost {
		return ErrHostKeyChanged
	}
	return h.accept(target, key, offered, prompt)
}

// accept は、未知のホストをどう扱うかを StrictHostKeyChecking で決める。
func (h HostKeys) accept(target Target, key ssh.PublicKey, offered string, prompt Prompter) error {
	switch target.Strict {
	case "yes":
		return ErrHostKeyUnknown
	case "no", "off", "accept-new":
		return h.remember(target, key, offered)
	}

	if prompt == nil {
		return ErrHostKeyUnknown
	}
	accepted, err := prompt.Confirm(UnknownHostPrompt(target, key))
	if err != nil {
		return err
	}
	if !accepted {
		return ErrHostKeyUnknown
	}
	return h.remember(target, key, offered)
}

func (h HostKeys) remember(target Target, key ssh.PublicKey, offered string) error {
	if h.Add == nil {
		return nil
	}
	port, err := strconv.Atoi(target.Port)
	if err != nil {
		port = 22
	}
	fingerprint, err := knownhosts.Fingerprint(offered)
	if err != nil {
		return err
	}
	return h.Add(knownhosts.Candidate{
		Host: target.HostName, Port: port, KeyType: key.Type(),
		Key: offered, Fingerprint: fingerprint,
	})
}

// UnknownHostPrompt は、OpenSSH が同じ場面で出す問いに寄せた文言である。
//
// フィンガープリントを見せるのは、そのユーザーが別の経路で確かめられるようにするため
// である。見せずに「続けますか」とだけ尋ねるのは、確かめる手段を持たない問いである。
func UnknownHostPrompt(target Target, key ssh.PublicKey) string {
	fingerprint := ssh.FingerprintSHA256(key)
	where := target.HostName
	if target.Alias != "" && target.Alias != target.HostName {
		where = target.Alias + " (" + target.HostName + ")"
	}
	return fmt.Sprintf(
		"The authenticity of host '%s' cannot be established.\r\n"+
			"%s key fingerprint is %s.\r\n"+
			"Are you sure you want to continue connecting (yes/no)? ",
		where, strings.ToUpper(key.Type()), fingerprint)
}

// hostField は、known_hosts がそのホストを書く形である。
//
// 既定のポートでは名前だけ、それ以外は [host]:port になる。この形は
// internal/knownhosts の Service.Add が書く形と同じでなければならない。
// 違えば、受け入れて書いた鍵に次の接続が一致しない。
func hostField(host, port string) string {
	if port == "" || port == "22" {
		return host
	}
	return "[" + host + "]:" + port
}
