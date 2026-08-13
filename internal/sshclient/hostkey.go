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
	// **ここだけは人に判断させない。** known_hosts にあって鍵が違うのは中間者攻撃の
	// 形そのものであり、「続けますか」と尋ねること自体が攻撃の成立条件になる。
	ErrHostKeyChanged = errors.New("the host key does not match the one in known_hosts")
	// ErrHostKeyUnknown は、未知のホストを受け入れなかったことを報告する。
	ErrHostKeyUnknown = errors.New("this host is not in known_hosts")
	// ErrHostKeyRevoked は、@revoked と印の付いた鍵を断る。
	ErrHostKeyRevoked = errors.New("this host key is marked revoked in known_hosts")
)

// HostKeys は、known_hosts との突き合わせである。
//
// 読むのも書くのも関数で受け取るのは、あの画面が読んでいるファイルと接続が読む
// ファイルを同じにするためである。**同じ問いに答えるものを二つ持たない。**
type HostKeys struct {
	// Read は known_hosts の中身を返す。nil なら、既知のホストは一つも無い。
	Read func() ([]byte, error)
	// Add は受け入れた鍵を書く。nil なら覚えない——接続はできるが、次も尋ねる。
	Add func(candidate knownhosts.Candidate) error
}

// Callback は、この接続のためのホスト鍵検証を返す。
//
// 問いを出す先を引数で受け取るのは、それが接続ごとに違うからである。**尋ねる
// のは、その接続を開いた端末でなければならない。** 別の端末に出た問いは、
// 誰も答えられないまま接続を止める。
func (h HostKeys) Callback(target Target, prompt Prompter) ssh.HostKeyCallback {
	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		return h.verify(target, key, prompt)
	}
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
// フィンガープリントを見せるのは、その人が別の経路で確かめられるようにするため
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
// 既定のポートでは名前だけ、それ以外は [host]:port になる。**この形は
// internal/knownhosts の Service.Add が書く形と同じでなければならない。**
// 違えば、受け入れて書いた鍵に次の接続が一致しない。
func hostField(host, port string) string {
	if port == "" || port == "22" {
		return host
	}
	return "[" + host + "]:" + port
}
