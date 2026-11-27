package sshclient_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sshc/internal/knownhosts"
	"sshc/internal/sshclient"
	"sshc/internal/storage"
	"sshc/internal/terminal"
)

// UserKnownHostsFile が ~/.ssh の外を指すときの、未知のホスト鍵の扱い。照合の読み取りと鍵の保存に
// 本物の knownhosts.Service を使い、どこへ書けてどこへ書けないかを本番と同じに決める。
//
// 2台のテストサーバーは別々のホスト鍵を持つ。HostKeyAlias で同じ名前（mirage）を付け、同じホストが
// 別の鍵を出した場面にする。

// testPassword は、テストサーバーが受け付けるパスワードである。
const testPassword = "hunter2"

// outsideKnownHostsHome は、~/.ssh と、その外にある known_hosts（~/.config/ssh/known_hosts）を持つ
// 一時のホームである。
type outsideKnownHostsHome struct {
	service *knownhosts.Service
	// outside は ~/.ssh の外の known_hosts である。空のファイルとして作る。
	outside string
}

func newOutsideKnownHostsHome(t *testing.T) outsideKnownHostsHome {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(home, ".config", "ssh", "known_hosts")
	if err := os.MkdirAll(filepath.Dir(outside), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	service := knownhosts.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), knownhosts.Scanner{})
	return outsideKnownHostsHome{service: service, outside: outside}
}

func (home outsideKnownHostsHome) dialer() sshclient.Dialer {
	return sshclient.Dialer{
		Auth:     sshclient.Auth{Password: func(sshclient.Target) (string, bool) { return testPassword, true }},
		HostKeys: sshclient.HostKeys{Read: home.service.ReadFile, Add: home.service.Remember},
	}
}

// outsideContents は、~/.ssh の外の known_hosts の今の中身である。
func (home outsideKnownHostsHome) outsideContents(t *testing.T) string {
	t.Helper()
	contents, err := os.ReadFile(home.outside)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}

// mirageAt は、このテストサーバーへ mirage として接続する Target である。
func mirageAt(server *testServer, strict string, userKnownHostsFiles []string) sshclient.Target {
	target := targetWith(server)
	target.Alias = "mirage"
	target.HostKeyAlias = "mirage"
	target.Strict = strict
	target.KnownHosts = sshclient.KnownHostsFiles{User: userKnownHostsFiles}
	return target
}

// connectionRoute は、StrictHostKeyChecking に従って未知のホスト鍵を受け入れる接続の経路である。
// SFTP の転送などの尋ねない接続は、未知の鍵を StrictHostKeyChecking に関わらず断るので含めない。
type connectionRoute struct {
	name string
	// connect は接続し、利用者に見せた失敗の文（ターミナルの出力）と、失敗の理由を返す。
	connect func(t *testing.T, dialer sshclient.Dialer, target sshclient.Target) (string, error)
}

var acceptingRoutes = []connectionRoute{
	{name: "terminal", connect: connectTerminal},
	{name: "SFTP tab", connect: connectSFTPTab},
}

// connectTerminal は、ターミナルのセッションとして接続し、つながったら閉じる。
func connectTerminal(t *testing.T, dialer sshclient.Dialer, target sshclient.Target) (string, error) {
	t.Helper()
	process, err := dialer.Open(context.Background(), target, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	copied := make(chan struct{})
	go func() {
		_, _ = io.Copy(&output, process)
		close(copied)
	}()
	readyErr := awaitReady(process)
	if readyErr == nil {
		_ = process.Close()
	}
	// 失敗したセッションは、失敗の文を書いてから出力を閉じる。
	<-copied
	_ = process.Close()
	return output.String(), readyErr
}

// connectSFTPTab は、SFTP のタブが最初の一覧を取る接続（見ている人に尋ねられる接続）として接続し、
// つながったら閉じる。ホスト鍵を尋ねられたら受け入れる。
func connectSFTPTab(t *testing.T, dialer sshclient.Dialer, target sshclient.Target) (string, error) {
	t.Helper()
	person := &watchingPerson{accept: true}
	connection, err := dialer.Connect(person.observe(context.Background()), target)
	if err != nil {
		return sshclient.FailureSentence(err), err
	}
	_ = connection.Close()
	return "", nil
}

// ~/.ssh の外の known_hosts には、sshc は鍵を書かない。accept-new と no で保存しないまま接続を続けると、
// 次の接続で別の鍵が来ても確認なしに受け入れるので、接続を失敗にする。失敗の文は、保存できない
// パスと、~/.ssh の中を指すか ssh で一度接続すれば登録できることを言う。
func TestAcceptNewAndNoFailInsteadOfTakingEveryKeyWhenKnownHostsIsOutsideSSH(t *testing.T) {
	for _, strict := range []string{"accept-new", "no"} {
		for _, route := range acceptingRoutes {
			t.Run(strict+" "+route.name, func(t *testing.T) {
				home := newOutsideKnownHostsHome(t)
				dialer := home.dialer()
				genuine := newTestServer(t, serverOptions{Password: testPassword})
				impostor := newTestServer(t, serverOptions{Password: testPassword})
				userKnownHostsFiles := []string{home.outside}

				sentence, first := route.connect(t, dialer, mirageAt(genuine, strict, userKnownHostsFiles))
				var outside *sshclient.KnownHostsOutsideWorkspaceError
				if !errors.As(first, &outside) || outside.Path != home.outside || !errors.Is(first, knownhosts.ErrNotWritable) {
					t.Errorf("first connection = %v, want a KnownHostsOutsideWorkspaceError for %s", first, home.outside)
				}
				if first != nil {
					for _, said := range []string{home.outside, "inside ~/.ssh", "connect once with ssh"} {
						if !strings.Contains(first.Error(), said) {
							t.Errorf("%q does not say %q", first, said)
						}
					}
				}
				for _, said := range []string{home.outside, "~/.sshの中のファイル", "sshで一度接続"} {
					if !strings.Contains(sentence, said) {
						t.Errorf("the person was told %q, which does not say %q", sentence, said)
					}
				}

				if _, second := route.connect(t, dialer, mirageAt(impostor, strict, userKnownHostsFiles)); second == nil {
					t.Errorf("a second connection with a different host key was accepted without asking")
				}
				if contents := home.outsideContents(t); contents != "" {
					t.Errorf("the known_hosts outside ~/.ssh = %q, want it untouched", contents)
				}
			})
		}
	}
}

// ssh で一度接続して ~/.ssh の外の known_hosts に鍵が入れば、sshc はその鍵と照合する。同じ鍵なら
// 接続し、別の鍵は変わった鍵として断る。
func TestAKeySavedBySSHOutsideSSHIsMatchedOnTheNextConnection(t *testing.T) {
	home := newOutsideKnownHostsHome(t)
	dialer := home.dialer()
	genuine := newTestServer(t, serverOptions{Password: testPassword})
	impostor := newTestServer(t, serverOptions{Password: testPassword})
	if err := os.WriteFile(home.outside, []byte(knownHostsLine("mirage", genuine.HostKey.PublicKey())), 0o600); err != nil {
		t.Fatal(err)
	}
	userKnownHostsFiles := []string{home.outside}

	for _, route := range acceptingRoutes {
		if _, err := route.connect(t, dialer, mirageAt(genuine, "accept-new", userKnownHostsFiles)); err != nil {
			t.Errorf("%s: the key ssh saved = %v, want it known", route.name, err)
		}
		if _, err := route.connect(t, dialer, mirageAt(impostor, "accept-new", userKnownHostsFiles)); !errors.Is(err, sshclient.ErrHostKeyChanged) {
			t.Errorf("%s: a different key = %v, want ErrHostKeyChanged", route.name, err)
		}
	}
}

// 保存しなくても今までどおり接続する設定。ask は接続のたびにフィンガープリントを見せて尋ねる。
// UserKnownHostsFile が /dev/null か none なのは、鍵を覚えないという利用者の指定である（OpenSSH と同じ）。
func TestConnectionsThatNeverSaveTheKeyStillConnect(t *testing.T) {
	for _, test := range []struct {
		name                string
		strict              string
		userKnownHostsFiles func(home outsideKnownHostsHome) []string
		asks                bool
	}{
		{name: "ask outside ~/.ssh", strict: "ask", userKnownHostsFiles: func(home outsideKnownHostsHome) []string { return []string{home.outside} }, asks: true},
		{name: "accept-new to the null device", strict: "accept-new", userKnownHostsFiles: func(outsideKnownHostsHome) []string { return []string{os.DevNull} }},
		{name: "accept-new with none", strict: "accept-new", userKnownHostsFiles: func(outsideKnownHostsHome) []string { return nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := newOutsideKnownHostsHome(t)
			server := newTestServer(t, serverOptions{Password: testPassword})
			person := &watchingPerson{accept: true}

			connection, err := home.dialer().Connect(person.observe(context.Background()), mirageAt(server, test.strict, test.userKnownHostsFiles(home)))
			if err != nil {
				t.Fatalf("Connect = %v", err)
			}
			_ = connection.Close()
			if asked := len(person.asked()) == 1; asked != test.asks {
				t.Errorf("asked %q, want a question: %v", person.asked(), test.asks)
			}
			if contents := home.outsideContents(t); contents != "" {
				t.Errorf("the known_hosts outside ~/.ssh = %q, want it untouched", contents)
			}
		})
	}
}
