package sshclient_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"sshc/internal/knownhosts"
	"sshc/internal/sshclient"
	"sshc/internal/terminal"
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
	}
	return recorder
}

// Confirm は、この記録係自身が Prompter として答える。
func (r *recordingHostKeys) Confirm(prompt string) (bool, error) {
	r.asked = append(r.asked, prompt)
	return r.answer, nil
}

func (r *recordingHostKeys) Line(string) (string, error)   { return "", sshclient.ErrPromptAborted }
func (r *recordingHostKeys) Secret(string) (string, error) { return "", sshclient.ErrPromptAborted }

type recordingHostKeys struct {
	sshclient.HostKeys
	added  []knownhosts.Candidate
	asked  []string
	answer bool
}

func verify(keys sshclient.HostKeys, target sshclient.Target, key ssh.PublicKey, prompt sshclient.Prompter) error {
	return keys.Callback(target, prompt)("ignored", &net.TCPAddr{}, key)
}

func TestAKnownHostWithTheSameKeyConnects(t *testing.T) {
	host := newHostKey(t)
	recorder := hostKeysFor(knownHostsLine("203.0.113.10", host.PublicKey()))
	target := sshclient.Target{Alias: "bastion", HostName: "203.0.113.10", Port: "22"}

	if err := verify(recorder.HostKeys, target, host.PublicKey(), recorder); err != nil {
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

	err := verify(recorder.HostKeys, target, offered.PublicKey(), recorder)
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

	if err := verify(recorder.HostKeys, target, host.PublicKey(), recorder); err != nil {
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

	if err := verify(recorder.HostKeys, target, host.PublicKey(), recorder); !errors.Is(err, sshclient.ErrHostKeyUnknown) {
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

		err := verify(recorder.HostKeys, target, host.PublicKey(), recorder)
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

	if err := verify(recorder.HostKeys, target, host.PublicKey(), recorder); err != nil {
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

	if err := verify(recorder.HostKeys, target, host.PublicKey(), recorder); !errors.Is(err, sshclient.ErrHostKeyUnknown) {
		t.Fatalf("verify = %v, want the port to make this a different host", err)
	}
}

func TestARevokedKeyIsRefused(t *testing.T) {
	host := newHostKey(t)
	recorder := hostKeysFor("@revoked " + knownHostsLine("203.0.113.10", host.PublicKey()))
	recorder.answer = true
	target := sshclient.Target{HostName: "203.0.113.10", Port: "22"}

	if err := verify(recorder.HostKeys, target, host.PublicKey(), recorder); !errors.Is(err, sshclient.ErrHostKeyRevoked) {
		t.Fatalf("verify = %v, want ErrHostKeyRevoked", err)
	}
}

// 尋ねる手段がなければ、未知のホストは断る。黙って受け入れることはしない。
func TestWithoutAWayToAskAnUnknownHostIsRefused(t *testing.T) {
	host := newHostKey(t)
	keys := sshclient.HostKeys{Read: func() ([]byte, error) { return nil, nil }}
	target := sshclient.Target{HostName: "203.0.113.10", Port: "22"}

	if err := verify(keys, target, host.PublicKey(), nil); !errors.Is(err, sshclient.ErrHostKeyUnknown) {
		t.Fatalf("verify = %v, want ErrHostKeyUnknown", err)
	}
}

// ホスト鍵の種類は、こちらがすでに持っているものを先に名乗る。
//
// **これが無いと、正しいホストの正しい鍵が「一致しない鍵」になる。** 普通の
// Ubuntu は ed25519 と ECDSA と RSA を持っており、x/crypto の既定表は ECDSA を
// ed25519 より前に置く。known_hosts にあるのが ed25519 の 1 行だけなら、返って
// くるのは known_hosts に無い種類の鍵であり、突き合わせは当然そこで終わる
// ——変わったのは相手ではなく、こちらの選び方である。
func TestTheKnownKeyTypeIsPreferredWhenTheHostOffersSeveral(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{public}, ExitCode: 42, ECDSAHostKey: true,
	})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}

	// dialerFor が書く known_hosts は、このサーバーの ed25519 鍵 1 行だけである。
	process, err := dialerFor(t, server, auth).Open(
		context.Background(), targetWith(server, path), terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()
	// **失敗したときも Wait が返るように、出力は読み続ける。** 繋げなかった理由は
	// 端末へ書かれるので、誰も読まないとその書き込みで止まる。
	go func() { _, _ = io.Copy(io.Discard, process) }()

	// **繋がったことは、向こうのシェルが終了コードを返したことで分かる。**
	// 握手がホスト鍵で終わっていれば、ここに来るのは別の数字である。
	if info := process.Wait(); info.Code != 42 {
		t.Fatalf("exit = %+v, want the shell on the other side to have run at all", info)
	}
}

// 知らないホストでは既定の順を返す。
//
// **何も渡さないと x/crypto の順になり、それは `ssh` の順ではない。** 初めて
// 繋ぐホストについて覚える鍵の種類が二つのクライアントで食い違うのは、同じ
// known_hosts を両方が書く以上、避けられるなら避けたい。
func TestAnUnknownHostGetsTheSameOrderOpenSSHWouldUse(t *testing.T) {
	recorder := hostKeysFor(knownHostsLine("203.0.113.10", newHostKey(t).PublicKey()))

	algorithms := recorder.Algorithms(sshclient.Target{HostName: "198.51.100.7", Port: "22"})
	if len(algorithms) == 0 || algorithms[0] != ssh.KeyAlgoED25519 {
		t.Errorf("algorithms = %#v, want ed25519 first", algorithms)
	}
	// 証明書は読まないので、名乗りもしない。受け取っても突き合わせられない。
	for _, algorithm := range algorithms {
		if strings.Contains(algorithm, "cert") {
			t.Errorf("a certificate algorithm was offered: %q", algorithm)
		}
	}
}

// 設定に書かれていれば、それが順序である。known_hosts が別の種類を持っていても
// 並べ替えない——人が決めた順を、こちらの都合で作り変えない。
func TestWhatTheConfigurationWroteWinsOverKnownHosts(t *testing.T) {
	recorder := hostKeysFor(knownHostsLine("203.0.113.10", newHostKey(t).PublicKey()))
	target := sshclient.Target{
		HostName: "203.0.113.10", Port: "22",
		HostKeyAlgorithms: []string{ssh.KeyAlgoRSASHA512},
	}

	algorithms := recorder.Algorithms(target)
	if len(algorithms) != 1 || algorithms[0] != ssh.KeyAlgoRSASHA512 {
		t.Errorf("algorithms = %#v, want exactly what the configuration wrote", algorithms)
	}
}

// RSA だけは、鍵の種類と署名アルゴリズムが一対一ではない。ssh-rsa としか
// 書かれていない 1 行から、同じ鍵で名乗れる三つを出す——SHA-1 だけを名乗ると、
// それを断るサーバーには繋がらない。
func TestAnRSAEntryOffersTheSHA2SignaturesToo(t *testing.T) {
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	public, err := ssh.NewPublicKey(&private.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	recorder := hostKeysFor(knownHostsLine("203.0.113.10", public))

	algorithms := recorder.Algorithms(sshclient.Target{HostName: "203.0.113.10", Port: "22"})
	want := []string{ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256, ssh.KeyAlgoRSA}
	if len(algorithms) != len(want) {
		t.Fatalf("algorithms = %#v, want %#v", algorithms, want)
	}
	for index, algorithm := range want {
		if algorithms[index] != algorithm {
			t.Fatalf("algorithms = %#v, want %#v", algorithms, want)
		}
	}
}
