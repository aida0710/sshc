package effective_test

import (
	"errors"
	"path/filepath"
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

// UserKnownHostsFile の用途として OpenSSH が示す ~/.ssh/known_hosts.d/%k などを
// 展開する。期待値は OpenSSH 10.2 の `ssh -G -o ProxyJump=u@j1:22,j2
// -o HostKeyAlias=KA -l ru -p 2222 dest.example` が同じホスト名のマシンで出した値である。
func TestExpandTokensHashesTheConnectionAndNamesTheHostKeyLikeOpenSSH(t *testing.T) {
	facts := effective.LocalFacts{Hostname: "dell-r540"}
	target := effective.TokenTarget{
		Alias: "dest.example", HostName: "dest.example", Port: "2222", RemoteUser: "ru",
		HostKeyAlias: "ka", JumpHost: "j2",
	}
	got, err := effective.ExpandTokens("/tmp/%C/%k/%j", facts, target)
	if want := "/tmp/f54f7b7bf4381397b32cc99bfe4cbbc3d32cc669/ka/j2"; err != nil || got != want {
		t.Errorf("ExpandTokens = %q, %v; want %q", got, err, want)
	}

	// HostKeyAlias が無ければ %k は利用者が打った名前である。
	withoutAlias := effective.TokenTarget{Alias: "Bastion", HostName: "203.0.113.10"}
	if got, err := effective.ExpandTokens("%k", facts, withoutAlias); err != nil || got != "Bastion" {
		t.Errorf("ExpandTokens(%%k) = %q, %v; want the alias", got, err)
	}
}

// UserKnownHostsFile と IdentityFile の ${NAME} は、OpenSSH と同じく ~ の後、
// トークンの前に環境変数へ置き換える。値の無い変数と壊れた書き方は断る。
func TestExpandFilePathExpandsEnvironmentVariablesBeforeTokens(t *testing.T) {
	facts := testFacts()
	facts.LookupEnv = func(name string) (string, bool) {
		value, ok := map[string]string{"KNOWN": testHome + "/known", "EMPTY": ""}[name]
		return value, ok
	}
	got, err := effective.ExpandFilePath("${KNOWN}${EMPTY}/%k", facts, testTarget())
	if want := filepath.Join(testHome, "known", "bastion"); err != nil || got != want {
		t.Errorf("ExpandFilePath = %q, %v; want %q", got, err, want)
	}
	for _, refused := range []string{"${MISSING}/known_hosts", "${KNOWN/known_hosts", "/${}/known_hosts"} {
		if _, err := effective.ExpandFilePath(refused, facts, testTarget()); !errors.Is(err, effective.ErrEnvironmentVariable) {
			t.Errorf("ExpandFilePath(%q) = %v, want ErrEnvironmentVariable", refused, err)
		}
	}
	// 接続先が決まる前に読む Keys の画面は、接続する側の環境に依る値を読まない。
	if _, err := effective.ExpandLocalFilePath("${KNOWN}/id", testHome); !errors.Is(err, effective.ErrUnknownToken) {
		t.Errorf("ExpandLocalFilePath = %v, want ErrUnknownToken", err)
	}
}

// OpenSSH の vdollar_percent_expand は ${NAME} と % を左から 1 回だけ読み、置き換えた
// 環境変数の値を読み直さない。値の中の % や ${ は文字どおりのファイル名になる。
func TestExpandFilePathKeepsPercentAndDollarInAnEnvironmentValueAsWritten(t *testing.T) {
	facts := testFacts()
	facts.LookupEnv = func(name string) (string, bool) {
		value, ok := map[string]string{"DIR": testHome + "/100%h", "NESTED": "${DIR}", "H": "h"}[name]
		return value, ok
	}
	for _, test := range []struct{ in, want string }{
		{"${DIR}/known", filepath.Join(testHome, "100%h", "known")},
		{"%d/${NESTED}", filepath.Join(testHome, "${DIR}")},
		{"%d/%%${H}", filepath.Join(testHome, "%h")},
	} {
		got, err := effective.ExpandFilePath(test.in, facts, testTarget())
		if err != nil || got != test.want {
			t.Errorf("ExpandFilePath(%q) = %q, %v; want %q", test.in, got, err, test.want)
		}
	}
	// % の直後の ${ はトークンではない。OpenSSH も未知のトークンとして断る。
	if _, err := effective.ExpandFilePath("%d/%${H}", facts, testTarget()); !errors.Is(err, effective.ErrUnknownToken) {
		t.Errorf("ExpandFilePath(%%${H}) = %v, want ErrUnknownToken", err)
	}
}

// Win32-OpenSSH の tilde_expand は、Windows では ~\ も ~/ と同じくホームに置き換える。
// Unix の OpenSSH は ~\ を「\」という名前のユーザーのホームとして読む。
func TestExpandFilePathReadsATildeBeforeThisSystemsSeparatorAsTheHome(t *testing.T) {
	written := "~" + string(filepath.Separator) + filepath.Join(".ssh", "known_hosts")
	got, err := effective.ExpandFilePath(written, testFacts(), testTarget())
	if want := filepath.Join(testHome, ".ssh", "known_hosts"); err != nil || got != want {
		t.Errorf("ExpandFilePath(%q) = %q, %v; want %q", written, got, err, want)
	}
	otherUsers := []string{"~other/.ssh/id_ed25519"}
	if filepath.Separator == '/' {
		otherUsers = append(otherUsers, `~\.ssh\id_ed25519`)
	}
	for _, written := range otherUsers {
		if _, err := effective.ExpandFilePath(written, testFacts(), testTarget()); !errors.Is(err, effective.ErrOtherUsersHome) {
			t.Errorf("ExpandFilePath(%q) = %v, want ErrOtherUsersHome", written, err)
		}
	}
}

// GlobalKnownHostsFile は ~ だけを展開する。% と ${ は文字どおりのファイル名である。
func TestExpandTildeFilePathLeavesTokensAndVariablesAsWritten(t *testing.T) {
	got, err := effective.ExpandTildeFilePath("~/global/%h/${HOME}", testHome)
	if want := filepath.Join(testHome, "global", "%h", "${HOME}"); err != nil || got != want {
		t.Errorf("ExpandTildeFilePath = %q, %v; want %q", got, err, want)
	}
	if _, err := effective.ExpandTildeFilePath("known_hosts", testHome); !errors.Is(err, effective.ErrRelativePath) {
		t.Errorf("ExpandTildeFilePath(relative) = %v, want ErrRelativePath", err)
	}
}
