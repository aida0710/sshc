package effective_test

import (
	"errors"
	"testing"

	"sshc/internal/effective"
)

func testFacts() effective.LocalFacts {
	return effective.LocalFacts{User: "aida", Home: testHome, Hostname: "mac.local", UID: "501"}
}

func testTarget() effective.TokenTarget {
	return effective.TokenTarget{
		Alias: "bastion", HostName: "203.0.113.10", Port: "2222", RemoteUser: "ops",
	}
}

func TestExpandTokensReplacesWhatOpenSSHReplaces(t *testing.T) {
	for _, test := range []struct{ in, want string }{
		{"~/.ssh/%h.key", "~/.ssh/203.0.113.10.key"},
		{"%n", "bastion"},
		{"%r@%h:%p", "ops@203.0.113.10:2222"},
		// 展開は差し込みだけで、パスを組み立て直さない。OpenSSH もそうする。残りは
		// 設定の構文のままであり、それをネイティブなパスに移すのは値を使う側である。
		{"%d/.ssh/id", testHome + "/.ssh/id"},
		{"%u/%i", "aida/501"},
		{"%l", "mac.local"},
		// %L は最初のドットまで。
		{"%L", "mac"},
		{"100%%", "100%"},
		// トークンを持たない文字列はそのまま返る。
		{"/etc/ssh/id_ed25519", "/etc/ssh/id_ed25519"},
	} {
		got, err := effective.ExpandTokens(test.in, testFacts(), testTarget())
		if err != nil || got != test.want {
			t.Errorf("ExpandTokens(%q) = %q, %v; want %q", test.in, got, err, test.want)
		}
	}
}

// 展開できないものを暗黙に残すと、その文字列はファイル名やコマンドとして
// そのまま使われる。応答しないことと、間違って返すことは別である。
func TestExpandTokensRefusesWhatItCannotAnswer(t *testing.T) {
	for _, refused := range []string{
		// %C は接続の材料からハッシュを作る。ここでは作らない。
		"%C",
		// %f と %T は接続の途中でしか意味を持たない。
		"%f", "%T",
		// 知らないトークン。
		"%z",
		// 末尾の単独の %。
		"trailing%",
	} {
		if _, err := effective.ExpandTokens(refused, testFacts(), testTarget()); !errors.Is(err, effective.ErrUnknownToken) {
			t.Errorf("ExpandTokens(%q) = %v, want ErrUnknownToken", refused, err)
		}
	}
}

// ドットを持たないホスト名では %L と %l が同じものになる。
func TestExpandTokensHandlesAHostnameWithoutADot(t *testing.T) {
	facts := effective.LocalFacts{Hostname: "mac"}
	for _, token := range []string{"%L", "%l"} {
		got, err := effective.ExpandTokens(token, facts, effective.TokenTarget{})
		if err != nil || got != "mac" {
			t.Errorf("ExpandTokens(%q) = %q, %v", token, got, err)
		}
	}
}
