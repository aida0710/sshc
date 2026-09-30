package sshclient_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"strings"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"

	"sshc/internal/knownhosts"
	"sshc/internal/sshclient"
	"sshc/internal/terminal"
)

// testKnownHosts は、テストの Target が照合に使う known_hosts である。偽の Read は
// パスを見ないので、名前はひとつあればよい。
var testKnownHosts = sshclient.KnownHostsFiles{User: []string{"known_hosts"}}

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
		Read: func(string) ([]byte, error) { return []byte(contents), nil },
		Add: func(_ string, candidate knownhosts.Candidate) error {
			recorder.added = append(recorder.added, candidate)
			return nil
		},
	}
	return recorder
}

// Confirm は、この記録係自身が Prompter として返す。
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
	target := sshclient.Target{Alias: "bastion", HostName: "203.0.113.10", Port: "22", KnownHosts: testKnownHosts}

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

// ここだけはユーザーに判断させない。known_hosts にあって鍵が違うのは中間者攻撃の
// 形そのものであり、尋ねること自体が攻撃の成立条件になる。
func TestAChangedHostKeyIsRefusedWithoutAsking(t *testing.T) {
	known, offered := newHostKey(t), newHostKey(t)
	recorder := hostKeysFor(knownHostsLine("203.0.113.10", known.PublicKey()))
	recorder.answer = true // yes を返せる状態にするが、このテストでは問い合わせ自体を禁止する。
	target := sshclient.Target{Alias: "bastion", HostName: "203.0.113.10", Port: "22", KnownHosts: testKnownHosts}

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
	target := sshclient.Target{Alias: "bastion", HostName: "203.0.113.10", Port: "2222", KnownHosts: testKnownHosts}

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
	target := sshclient.Target{HostName: "203.0.113.10", Port: "22", KnownHosts: testKnownHosts}

	if err := verify(recorder.HostKeys, target, host.PublicKey(), recorder); !errors.Is(err, sshclient.ErrHostKeyUnknown) {
		t.Fatalf("verify = %v, want ErrHostKeyUnknown", err)
	}
	if len(recorder.added) != 0 {
		t.Fatalf("a refused host key was written: %#v", recorder.added)
	}
}

func TestStrictHostKeyCheckingDecidesWhatHappensToAnUnknownHost(t *testing.T) {
	for _, test := range []struct {
		strict     string
		wantAsk    bool
		wantAdd    bool
		wantFail   bool
		fromConfig bool
	}{
		{strict: "", wantAsk: true, wantAdd: true},
		{strict: "ask", wantAsk: true, wantAdd: true},
		{strict: "accept-new", wantAdd: true},
		{strict: "no", wantAdd: true},
		{strict: "yes", wantFail: true},
		// 設定の true と false は、NewTarget が yes と no に揃えてから届く。
		{strict: "true", wantFail: true, fromConfig: true},
		{strict: "false", wantAdd: true, fromConfig: true},
	} {
		host := newHostKey(t)
		recorder := hostKeysFor("")
		recorder.answer = true
		target := sshclient.Target{HostName: "203.0.113.10", Port: "22", Strict: test.strict, KnownHosts: testKnownHosts}
		if test.fromConfig {
			built, err := sshclient.NewTarget("web", resolverFor(map[string]map[string][]string{
				"web": {"hostname": {"203.0.113.10"}, "port": {"22"}, "stricthostkeychecking": {test.strict}},
			}), testFacts)
			if err != nil {
				t.Fatal(err)
			}
			target = built
		}

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

// x/crypto は、接続中の鍵の再交換（rekey）のたびにも同じ callback を呼ぶ。受け入れた
// 鍵を rekey で尋ね直すと、問いがセッションの途中に出て、利用者の打鍵を答えとして読む。
func TestAnAcceptedHostKeyIsNotAskedOrWrittenAgainWhenTheKeysAreExchangedAgain(t *testing.T) {
	for _, test := range []struct {
		strict    string
		wantAsked int
	}{
		{strict: "ask", wantAsked: 1},
		{strict: "accept-new"},
		{strict: "no"},
	} {
		host := newHostKey(t)
		recorder := hostKeysFor("")
		recorder.answer = true
		target := sshclient.Target{HostName: "203.0.113.10", Port: "22", Strict: test.strict, KnownHosts: testKnownHosts}
		callback := recorder.Callback(target, recorder)

		for exchange := 1; exchange <= 2; exchange++ {
			if err := callback("ignored", &net.TCPAddr{}, host.PublicKey()); err != nil {
				t.Fatalf("StrictHostKeyChecking %s: key exchange %d = %v", test.strict, exchange, err)
			}
		}
		if len(recorder.asked) != test.wantAsked {
			t.Errorf("StrictHostKeyChecking %s asked %d times, want %d", test.strict, len(recorder.asked), test.wantAsked)
		}
		if len(recorder.added) != 1 {
			t.Errorf("StrictHostKeyChecking %s wrote %d keys, want 1", test.strict, len(recorder.added))
		}
	}
}

// rekey では known_hosts と照合し直さず、最初の鍵交換と同じ鍵であることを求める。
// known_hosts に両方の鍵があっても、接続の途中で鍵を替えたホストは断る。
func TestAHostKeyThatChangesWhenTheKeysAreExchangedAgainIsRefused(t *testing.T) {
	first, second := newHostKey(t), newHostKey(t)
	recorder := hostKeysFor(knownHostsLine("203.0.113.10", first.PublicKey()) + knownHostsLine("203.0.113.10", second.PublicKey()))
	recorder.answer = true
	target := sshclient.Target{HostName: "203.0.113.10", Port: "22", KnownHosts: testKnownHosts}
	callback := recorder.Callback(target, recorder)

	if err := callback("ignored", &net.TCPAddr{}, first.PublicKey()); err != nil {
		t.Fatalf("first key exchange = %v", err)
	}
	if err := callback("ignored", &net.TCPAddr{}, second.PublicKey()); !errors.Is(err, sshclient.ErrHostKeyChangedDuringRekey) {
		t.Fatalf("rekey with another key = %v, want ErrHostKeyChangedDuringRekey", err)
	}
	if len(recorder.asked) != 0 || len(recorder.added) != 0 {
		t.Errorf("asked = %#v, added = %#v, want neither", recorder.asked, recorder.added)
	}
}

// 既定でないポートのホストは [host]:port として保存される。この形が
// internal/knownhosts の書く形とずれると、受け入れて書いた鍵に次の接続が一致しない。
func TestANonDefaultPortMatchesTheBracketedForm(t *testing.T) {
	host := newHostKey(t)
	recorder := hostKeysFor(knownHostsLine("[203.0.113.10]:2222", host.PublicKey()))
	target := sshclient.Target{HostName: "203.0.113.10", Port: "2222", KnownHosts: testKnownHosts}

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
	target := sshclient.Target{HostName: "203.0.113.10", Port: "2222", KnownHosts: testKnownHosts}

	if err := verify(recorder.HostKeys, target, host.PublicKey(), recorder); !errors.Is(err, sshclient.ErrHostKeyUnknown) {
		t.Fatalf("verify = %v, want the port to make this a different host", err)
	}
}

func TestARevokedKeyIsRefused(t *testing.T) {
	host := newHostKey(t)
	recorder := hostKeysFor("@revoked " + knownHostsLine("203.0.113.10", host.PublicKey()))
	recorder.answer = true
	target := sshclient.Target{HostName: "203.0.113.10", Port: "22", KnownHosts: testKnownHosts}

	if err := verify(recorder.HostKeys, target, host.PublicKey(), recorder); !errors.Is(err, sshclient.ErrHostKeyRevoked) {
		t.Fatalf("verify = %v, want ErrHostKeyRevoked", err)
	}
}

// 尋ねる手段がなければ、未知のホストは断る。暗黙に受け入れることはしない。
func TestWithoutAWayToAskAnUnknownHostIsRefused(t *testing.T) {
	host := newHostKey(t)
	keys := sshclient.HostKeys{Read: func(string) ([]byte, error) { return nil, nil }}
	target := sshclient.Target{HostName: "203.0.113.10", Port: "22", KnownHosts: testKnownHosts}

	if err := verify(keys, target, host.PublicKey(), nil); !errors.Is(err, sshclient.ErrHostKeyUnknown) {
		t.Fatalf("verify = %v, want ErrHostKeyUnknown", err)
	}
}

// ホスト鍵の種類は、こちらがすでに持っているものを先に名乗る。
//
// これが無いと、正しいホストの正しい鍵が「一致しない鍵」になる。普通の
// Ubuntu は ed25519 と ECDSA と RSA を持っており、x/crypto の既定表は ECDSA を
// ed25519 より前に置く。known_hosts にあるのが ed25519 の 1 行だけなら、返って
// くるのは known_hosts に無い種類の鍵であり、突き合わせは当然そこで終わる
// 変わったのは相手ではなく、こちらの選び方である。
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
	// 失敗したときも Wait が返るように、出力は読み続ける。繋げなかった理由は
	// 端末へ書かれるので、誰も読まないとその書き込みで止まる。
	go func() { _, _ = io.Copy(io.Discard, process) }()

	// 繋がったことは、向こうのシェルが終了コードを返したことで分かる。
	// 握手がホスト鍵で終わっていれば、ここに来るのは別の数字である。
	if info := process.Wait(); info.Code != 42 {
		t.Fatalf("exit = %+v, want the shell on the other side to have run at all", info)
	}
}

// 知らないホストでは既定の順を返す。
//
// 何も渡さないと x/crypto の順になり、それは `ssh` の順ではない。初めて
// 繋ぐホストについて覚える鍵の種類が二つのクライアントで食い違うのは、同じ
// known_hosts を両方が書く以上、避けられるなら避けたい。
func TestAnUnknownHostGetsTheSameOrderOpenSSHWouldUse(t *testing.T) {
	recorder := hostKeysFor(knownHostsLine("203.0.113.10", newHostKey(t).PublicKey()))

	algorithms := recorder.Algorithms(sshclient.Target{HostName: "198.51.100.7", Port: "22", KnownHosts: testKnownHosts})
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
// 並べ替えない。ユーザーが決めた順を、こちらの都合で作り変えない。
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
// 書かれていない 1 行から、同じ鍵で名乗れるSHA-2の二つを出す。SHA-1を名乗ると、
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

	algorithms := recorder.Algorithms(sshclient.Target{HostName: "203.0.113.10", Port: "22", KnownHosts: testKnownHosts})
	want := []string{ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256}
	if len(algorithms) != len(want) {
		t.Fatalf("algorithms = %#v, want %#v", algorithms, want)
	}
	for index, algorithm := range want {
		if algorithms[index] != algorithm {
			t.Fatalf("algorithms = %#v, want %#v", algorithms, want)
		}
	}
}

// fakeKnownHostsFiles は、パスごとに中身を返す偽の known_hosts である。
type fakeKnownHostsFiles struct {
	contents map[string]string
	written  map[string][]knownhosts.Candidate
	refuse   error
}

func (files *fakeKnownHostsFiles) hostKeys() sshclient.HostKeys {
	return sshclient.HostKeys{
		Read: func(path string) ([]byte, error) { return []byte(files.contents[path]), nil },
		Add: func(path string, candidate knownhosts.Candidate) error {
			if files.refuse != nil {
				return files.refuse
			}
			if files.written == nil {
				files.written = map[string][]knownhosts.Candidate{}
			}
			files.written[path] = append(files.written[path], candidate)
			return nil
		},
	}
}

var separateKnownHosts = sshclient.KnownHostsFiles{
	User:   []string{"/home/me/.ssh/known_hosts", "/home/me/.ssh/known_hosts2"},
	Global: []string{"/etc/ssh/ssh_known_hosts"},
}

// 組織が GlobalKnownHostsFile で配った鍵と違う鍵は、accept-new でも受け入れない。
// 未知のホストとして扱うと、偽の鍵を ~/.ssh/known_hosts に書き、以後の ssh も通す。
func TestAKeyThatDiffersFromTheGlobalKnownHostsIsRefusedEvenWithAcceptNew(t *testing.T) {
	distributed, offered := newHostKey(t), newHostKey(t)
	files := &fakeKnownHostsFiles{contents: map[string]string{
		"/etc/ssh/ssh_known_hosts": knownHostsLine("db.example", distributed.PublicKey()),
	}}
	target := sshclient.Target{HostName: "db.example", Port: "22", Strict: "accept-new", KnownHosts: separateKnownHosts}

	if err := verify(files.hostKeys(), target, offered.PublicKey(), nil); !errors.Is(err, sshclient.ErrHostKeyChanged) {
		t.Fatalf("verify = %v, want ErrHostKeyChanged", err)
	}
	if len(files.written) != 0 {
		t.Fatalf("a key that contradicts the global file was written: %#v", files.written)
	}
	if err := verify(files.hostKeys(), target, distributed.PublicKey(), nil); err != nil {
		t.Fatalf("the distributed key = %v, want it known", err)
	}
}

// どのファイルにある @revoked も、ほかのファイルの同じ鍵の行より強い。
func TestARevokedKeyInAnyFileIsRefused(t *testing.T) {
	host := newHostKey(t)
	files := &fakeKnownHostsFiles{contents: map[string]string{
		"/home/me/.ssh/known_hosts": knownHostsLine("db.example", host.PublicKey()),
		"/etc/ssh/ssh_known_hosts":  "@revoked " + knownHostsLine("db.example", host.PublicKey()),
	}}
	target := sshclient.Target{HostName: "db.example", Port: "22", KnownHosts: separateKnownHosts}

	if err := verify(files.hostKeys(), target, host.PublicKey(), nil); !errors.Is(err, sshclient.ErrHostKeyRevoked) {
		t.Fatalf("verify = %v, want ErrHostKeyRevoked", err)
	}
}

// HostKeyAlias があれば、HostName と Port ではなくその名前で照合し、その名前で書く。
// 踏み台越しに localhost:2222 へ繋ぐ構成で、別のホストの鍵と混ざらない。
func TestHostKeyAliasNamesTheHostInKnownHosts(t *testing.T) {
	known, offered := newHostKey(t), newHostKey(t)
	files := &fakeKnownHostsFiles{contents: map[string]string{
		"/home/me/.ssh/known_hosts": knownHostsLine("myalias", known.PublicKey()),
	}}
	target := sshclient.Target{
		HostName: "localhost", Port: "2222", HostKeyAlias: "myalias", Strict: "accept-new", KnownHosts: separateKnownHosts,
	}

	if err := verify(files.hostKeys(), target, offered.PublicKey(), nil); !errors.Is(err, sshclient.ErrHostKeyChanged) {
		t.Fatalf("verify = %v, want ErrHostKeyChanged against the alias line", err)
	}
	if err := verify(files.hostKeys(), target, known.PublicKey(), nil); err != nil {
		t.Fatalf("the alias key = %v, want it known", err)
	}

	fresh := &fakeKnownHostsFiles{}
	if err := verify(fresh.hostKeys(), target, offered.PublicKey(), nil); err != nil {
		t.Fatal(err)
	}
	written := fresh.written["/home/me/.ssh/known_hosts"]
	if len(written) != 1 || knownhosts.HostField(written[0].Host, written[0].Port) != "myalias" {
		t.Fatalf("written = %#v, want the key under myalias", fresh.written)
	}
}

// 受け入れた鍵は UserKnownHostsFile の最初のファイルへ書く。none なら書かない。
// sshc が書かない場所（~/.ssh の外、/dev/null）なら、書かずに接続は続ける。
func TestAnAcceptedKeyGoesToTheFirstUserKnownHostsFile(t *testing.T) {
	host := newHostKey(t)
	for _, test := range []struct {
		name   string
		user   []string
		refuse error
		want   string
	}{
		{name: "first file", user: []string{"/home/me/.ssh/work_hosts", "/home/me/.ssh/known_hosts"}, want: "/home/me/.ssh/work_hosts"},
		{name: "none", user: nil},
		{name: "outside the workspace", user: []string{"/dev/null"}, refuse: knownhosts.ErrNotWritable},
	} {
		files := &fakeKnownHostsFiles{refuse: test.refuse}
		target := sshclient.Target{
			HostName: "new.example", Port: "22", Strict: "no",
			KnownHosts: sshclient.KnownHostsFiles{User: test.user},
		}
		if err := verify(files.hostKeys(), target, host.PublicKey(), nil); err != nil {
			t.Fatalf("%s: verify = %v", test.name, err)
		}
		switch {
		case test.want == "" && len(files.written) != 0:
			t.Errorf("%s: written = %#v, want nothing", test.name, files.written)
		case test.want != "" && len(files.written[test.want]) != 1:
			t.Errorf("%s: written = %#v, want one key in %s", test.name, files.written, test.want)
		}
	}
}

// dotfiles の管理で ~/.ssh/known_hosts をリンクにしていると、受け入れた鍵を保存できない。
// 保存しないまま続けると accept-new が毎回確認なしで受け入れるので、接続は失敗にする。
// 失敗の文は、リンクのためであることと、ssh で一度接続すれば登録できることを言う。
func TestAKeyThatCannotBeSavedThroughASymlinkFailsTheConnectionAndSaysHowToAddIt(t *testing.T) {
	host := newHostKey(t)
	linked := "/home/me/.ssh/known_hosts"
	for _, strict := range []string{"accept-new", "ask"} {
		files := &fakeKnownHostsFiles{refuse: fmt.Errorf("%w: %s", knownhosts.ErrSymlinkPath, linked)}
		target := sshclient.Target{
			HostName: "new.example", Port: "22", Strict: strict,
			KnownHosts: sshclient.KnownHostsFiles{User: []string{linked}},
		}
		confirmed := &recordingHostKeys{answer: true}

		err := verify(files.hostKeys(), target, host.PublicKey(), confirmed)
		var symlinked *sshclient.KnownHostsSymlinkError
		if !errors.As(err, &symlinked) || symlinked.Path != linked || !errors.Is(err, knownhosts.ErrSymlinkPath) {
			t.Fatalf("%s: verify = %v, want a KnownHostsSymlinkError for %s", strict, err, linked)
		}
		for _, said := range []string{linked, "symbolic link", "connect once with ssh"} {
			if !strings.Contains(err.Error(), said) {
				t.Errorf("%s: %q does not say %q", strict, err, said)
			}
		}
	}
}

// 名乗るホスト鍵アルゴリズムの選択と鍵の照合は、同じ読み取りを使う。組織が配る
// 大きな GlobalKnownHostsFile を、接続のたびに二度読んで解析しない。
func TestAConnectionReadsEachKnownHostsFileOnce(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{public}})
	known := knownHostsLine("["+server.Host()+"]:"+server.Port(), server.HostKey.PublicKey())
	var mutex sync.Mutex
	reads := map[string]int{}
	dialer := sshclient.Dialer{
		Auth: sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }},
		HostKeys: sshclient.HostKeys{Read: func(file string) ([]byte, error) {
			mutex.Lock()
			defer mutex.Unlock()
			reads[file]++
			if file == "ssh_known_hosts" {
				return []byte(known), nil
			}
			return nil, nil
		}},
	}
	target := targetWith(server, path)
	target.KnownHosts = sshclient.KnownHostsFiles{User: []string{"known_hosts"}, Global: []string{"ssh_known_hosts"}}

	connection, err := dialer.Connect(context.Background(), target)
	if err != nil {
		t.Fatalf("Connect = %v", err)
	}
	_ = connection.Close()
	if _, err := dialer.Probe(context.Background(), target); err != nil {
		t.Fatalf("Probe = %v", err)
	}

	mutex.Lock()
	defer mutex.Unlock()
	if want := map[string]int{"known_hosts": 2, "ssh_known_hosts": 2}; !maps.Equal(reads, want) {
		t.Errorf("reads over two connections = %v, want %v", reads, want)
	}
}
