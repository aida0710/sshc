package sshclient_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"sshc/internal/knownhosts"
	"sshc/internal/sshclient"
)

func newHostKey(t *testing.T) ssh.Signer {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

// knownHostsLine は、この鍵をそのホストについて書いた 1 行である。
func knownHostsLine(host string, key ssh.PublicKey) string {
	return host + " " + key.Type() + " " + base64.StdEncoding.EncodeToString(key.Marshal()) + "\n"
}

func hostKeysFor(contents string) *recordingHostKeys {
	recorder := &recordingHostKeys{}
	recorder.HostKeys = sshclient.HostKeys{
		Read: func() ([]byte, error) { return []byte(contents), nil },
		Add: func(candidate knownhosts.Candidate) error {
			recorder.added = append(recorder.added, candidate)
			return nil
		},
		Ask: func(prompt string) (bool, error) {
			recorder.asked = append(recorder.asked, prompt)
			return recorder.answer, nil
		},
	}
	return recorder
}

type recordingHostKeys struct {
	sshclient.HostKeys
	added  []knownhosts.Candidate
	asked  []string
	answer bool
}

func verify(keys sshclient.HostKeys, target sshclient.Target, key ssh.PublicKey) error {
	return keys.Callback(target)("ignored", &net.TCPAddr{}, key)
}

func TestAKnownHostWithTheSameKeyConnects(t *testing.T) {
	host := newHostKey(t)
	recorder := hostKeysFor(knownHostsLine("203.0.113.10", host.PublicKey()))
	target := sshclient.Target{Alias: "bastion", HostName: "203.0.113.10", Port: "22"}

	if err := verify(recorder.HostKeys, target, host.PublicKey()); err != nil {
		t.Fatalf("verify = %v", err)
	}
	if len(recorder.asked) != 0 {
		t.Errorf("a known host was still put to the user: %#v", recorder.asked)
	}
	if len(recorder.added) != 0 {
		t.Errorf("a known host was written again: %#v", recorder.added)
	}
}

// **ここだけは人に判断させない。** known_hosts にあって鍵が違うのは中間者攻撃の
// 形そのものであり、尋ねること自体が攻撃の成立条件になる。
func TestAChangedHostKeyIsRefusedWithoutAsking(t *testing.T) {
	known, offered := newHostKey(t), newHostKey(t)
	recorder := hostKeysFor(knownHostsLine("203.0.113.10", known.PublicKey()))
	recorder.answer = true // 尋ねられたら「はい」と答える用意がある。尋ねてはならない。
	target := sshclient.Target{Alias: "bastion", HostName: "203.0.113.10", Port: "22"}

	err := verify(recorder.HostKeys, target, offered.PublicKey())
	if !errors.Is(err, sshclient.ErrHostKeyChanged) {
		t.Fatalf("verify = %v, want ErrHostKeyChanged", err)
	}
	if len(recorder.asked) != 0 {
		t.Fatalf("a changed host key was put to the user: %#v", recorder.asked)
	}
	if len(recorder.added) != 0 {
		t.Fatalf("a changed host key was written: %#v", recorder.added)
	}
}

func TestAnUnknownHostIsPutToTheUserAndRememberedWhenAccepted(t *testing.T) {
	host := newHostKey(t)
	recorder := hostKeysFor("")
	recorder.answer = true
	target := sshclient.Target{Alias: "bastion", HostName: "203.0.113.10", Port: "2222"}

	if err := verify(recorder.HostKeys, target, host.PublicKey()); err != nil {
		t.Fatalf("verify = %v", err)
	}
	if len(recorder.asked) != 1 {
		t.Fatalf("asked = %#v", recorder.asked)
	}
	// フィンガープリントを見せる。見せずに尋ねるのは、確かめる手段を持たない問いである。
	if !strings.Contains(recorder.asked[0], ssh.FingerprintSHA256(host.PublicKey())) {
		t.Errorf("the prompt does not carry the fingerprint: %q", recorder.asked[0])
	}
	if len(recorder.added) != 1 || recorder.added[0].Port != 2222 {
		t.Fatalf("added = %#v", recorder.added)
	}
}

func TestAnUnknownHostIsRefusedWhenTheUserSaysNo(t *testing.T) {
	host := newHostKey(t)
	recorder := hostKeysFor("")
	recorder.answer = false
	target := sshclient.Target{HostName: "203.0.113.10", Port: "22"}

	if err := verify(recorder.HostKeys, target, host.PublicKey()); !errors.Is(err, sshclient.ErrHostKeyUnknown) {
		t.Fatalf("verify = %v, want ErrHostKeyUnknown", err)
	}
	if len(recorder.added) != 0 {
		t.Fatalf("a refused host key was written: %#v", recorder.added)
	}
}

func TestStrictHostKeyCheckingDecidesWhatHappensToAnUnknownHost(t *testing.T) {
	for _, test := range []struct {
		strict   string
		wantAsk  bool
		wantAdd  bool
		wantFail bool
	}{
		{strict: "", wantAsk: true, wantAdd: true},
		{strict: "ask", wantAsk: true, wantAdd: true},
		{strict: "accept-new", wantAdd: true},
		{strict: "no", wantAdd: true},
		{strict: "yes", wantFail: true},
	} {
		host := newHostKey(t)
		recorder := hostKeysFor("")
		recorder.answer = true
		target := sshclient.Target{HostName: "203.0.113.10", Port: "22", Strict: test.strict}

		err := verify(recorder.HostKeys, target, host.PublicKey())
		switch {
		case test.wantFail && !errors.Is(err, sshclient.ErrHostKeyUnknown):
			t.Errorf("StrictHostKeyChecking %q = %v, want ErrHostKeyUnknown", test.strict, err)
		case !test.wantFail && err != nil:
			t.Errorf("StrictHostKeyChecking %q = %v", test.strict, err)
		}
		if asked := len(recorder.asked) > 0; asked != test.wantAsk {
			t.Errorf("StrictHostKeyChecking %q asked = %v, want %v", test.strict, asked, test.wantAsk)
		}
		if added := len(recorder.added) > 0; added != test.wantAdd {
			t.Errorf("StrictHostKeyChecking %q added = %v, want %v", test.strict, added, test.wantAdd)
		}
	}
}

// 既定でないポートのホストは [host]:port として保存される。この形が
// internal/knownhosts の書く形とずれると、受け入れて書いた鍵に次の接続が一致しない。
func TestANonDefaultPortMatchesTheBracketedForm(t *testing.T) {
	host := newHostKey(t)
	recorder := hostKeysFor(knownHostsLine("[203.0.113.10]:2222", host.PublicKey()))
	target := sshclient.Target{HostName: "203.0.113.10", Port: "2222"}

	if err := verify(recorder.HostKeys, target, host.PublicKey()); err != nil {
		t.Fatalf("verify = %v", err)
	}
	if len(recorder.asked) != 0 {
		t.Errorf("a known host on a non-default port was still put to the user")
	}
}

// 既定でないポートの接続は、括弧なしの行に一致してはならない。あれは 22 番の
// ホストについての記録である。
func TestANonDefaultPortDoesNotMatchThePlainForm(t *testing.T) {
	host := newHostKey(t)
	recorder := hostKeysFor(knownHostsLine("203.0.113.10", host.PublicKey()))
	recorder.answer = false
	target := sshclient.Target{HostName: "203.0.113.10", Port: "2222"}

	if err := verify(recorder.HostKeys, target, host.PublicKey()); !errors.Is(err, sshclient.ErrHostKeyUnknown) {
		t.Fatalf("verify = %v, want the port to make this a different host", err)
	}
}

func TestARevokedKeyIsRefused(t *testing.T) {
	host := newHostKey(t)
	recorder := hostKeysFor("@revoked " + knownHostsLine("203.0.113.10", host.PublicKey()))
	recorder.answer = true
	target := sshclient.Target{HostName: "203.0.113.10", Port: "22"}

	if err := verify(recorder.HostKeys, target, host.PublicKey()); !errors.Is(err, sshclient.ErrHostKeyRevoked) {
		t.Fatalf("verify = %v, want ErrHostKeyRevoked", err)
	}
}

// 尋ねる手段がなければ、未知のホストは断る。黙って受け入れることはしない。
func TestWithoutAWayToAskAnUnknownHostIsRefused(t *testing.T) {
	host := newHostKey(t)
	keys := sshclient.HostKeys{Read: func() ([]byte, error) { return nil, nil }}
	target := sshclient.Target{HostName: "203.0.113.10", Port: "22"}

	if err := verify(keys, target, host.PublicKey()); !errors.Is(err, sshclient.ErrHostKeyUnknown) {
		t.Fatalf("verify = %v, want ErrHostKeyUnknown", err)
	}
}
