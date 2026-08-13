package platform_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"sshc/internal/platform"
)

func keyPassphraseCredential() platform.AskpassCredential {
	return platform.AskpassCredential{
		Helper:       "/Users/tester/.local/bin/sshc",
		URL:          "http://127.0.0.1:1/askpass",
		Token:        "the-one-time-token",
		Kind:         platform.AskpassKindKeyPassphrase,
		IdentityFile: "/Users/tester/.ssh/id_server",
		SSHConfig:    "Host bastion\n\tHostName example.invalid\n",
	}
}

func environmentByName(entries []string) map[string][]string {
	counted := map[string][]string{}
	for _, entry := range entries {
		name, value, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		counted[name] = append(counted[name], value)
	}
	return counted
}

// ここで設定する変数は、OpenSSH が読むものと一致していなければならない。
//
// exec.Cmd は配列をそのまま渡し、getenv はその中で最初に一致したものを返す。
// したがって継承した環境に追記する方式では、ユーザーが何年も前にシェルのプロファイル
// でエクスポートした SSH_ASKPASS に負ける — しかも負けながら、保存済み鍵パスフレーズと
// 引き換えられるワンタイムトークンをそのプログラムに渡してしまう。この攻撃の敷居は、
// エクスポートされた変数ひとつである。
func TestInteractiveSSHReplacesWhatItSetsRatherThanAppending(t *testing.T) {
	inherited := []string{
		"HOME=/Users/tester",
		"SSH_ASKPASS=/tmp/not-ours",
		"SSH_ASKPASS_REQUIRE=never",
		"SSHC_ASKPASS_TOKEN=stale",
		"PATH=/usr/bin",
	}
	session, cleanup, err := platform.InteractiveSSH(platform.InteractiveRequest{
		SSH: "/usr/bin/ssh", Alias: "bastion", Inherited: inherited,
		Credential: keyPassphraseCredential(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if !session.Armed || session.Notice != "" {
		t.Fatalf("Armed = %v, Notice = %q", session.Armed, session.Notice)
	}

	counted := environmentByName(session.Env)
	for name, want := range map[string]string{
		"SSH_ASKPASS":                   "/Users/tester/.local/bin/sshc",
		"SSH_ASKPASS_REQUIRE":           "force",
		platform.AskpassURLVariable:     "http://127.0.0.1:1/askpass",
		platform.AskpassTokenVariable:   "the-one-time-token",
		platform.AskpassAliasVariable:   "bastion",
		platform.AskpassKindVariable:    platform.AskpassKindKeyPassphrase,
		platform.AskpassKeyPathVariable: "/Users/tester/.ssh/id_server",
	} {
		if len(counted[name]) != 1 {
			t.Errorf("%s appears %d times: %v", name, len(counted[name]), counted[name])
			continue
		}
		if counted[name][0] != want {
			t.Errorf("%s = %q, want %q", name, counted[name][0], want)
		}
	}
	// それ以外にユーザーが持っていたものはすべてそのまま本人のものだ。これは自分で ssh を
	// 打ったときに得たであろう環境に、こちらが決めた五つを加えたものである。
	if len(counted["HOME"]) != 1 || counted["HOME"][0] != "/Users/tester" {
		t.Errorf("HOME = %v", counted["HOME"])
	}
	if len(counted["PATH"]) != 1 {
		t.Errorf("PATH = %v", counted["PATH"])
	}
}

func TestOnlyKeyPassphraseConnectionsUseTheFrozenConfiguration(t *testing.T) {
	armed, cleanup, err := platform.InteractiveSSH(platform.InteractiveRequest{
		SSH: "/usr/bin/ssh", Alias: "bastion", Credential: keyPassphraseCredential(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	// 凍結した設定のパスは実行時に決まるので、その位置だけを見て残りを比較する。
	if len(armed.Arguments) != 8 || armed.Arguments[0] != "-F" {
		t.Fatalf("armed arguments = %#v", armed.Arguments)
	}
	rest := append([]string{}, armed.Arguments[2:]...)
	if !slices.Equal(rest, []string{"-i", "/Users/tester/.ssh/id_server", "-o", "IdentitiesOnly=yes", "--", "bastion"}) {
		t.Fatalf("armed arguments = %#v", armed.Arguments)
	}

	plain, plainCleanup, err := platform.InteractiveSSH(platform.InteractiveRequest{
		SSH: "/usr/bin/ssh", Alias: "bastion",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer plainCleanup()
	if !slices.Equal(plain.Arguments, []string{"--", "bastion"}) {
		t.Fatalf("unarmed arguments = %#v", plain.Arguments)
	}
	if plain.Armed {
		t.Fatal("a connection with no credential reported itself armed")
	}
}

// トークンがなければ何も武装されない。ユーザーの環境に残った古い変数もまた、それを
// 武装させてはならない。
func TestAnUnarmedConnectionDropsStaleArming(t *testing.T) {
	session, cleanup, err := platform.InteractiveSSH(platform.InteractiveRequest{
		SSH: "/usr/bin/ssh", Alias: "bastion",
		Inherited: []string{"SSH_ASKPASS=/tmp/not-ours", "SSHC_ASKPASS_TOKEN=stale", "HOME=/Users/tester"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	for _, entry := range session.Env {
		for _, name := range []string{
			"SSH_ASKPASS=", platform.AskpassTokenVariable + "=",
			platform.AskpassURLVariable + "=", platform.AskpassAliasVariable + "=",
		} {
			if strings.HasPrefix(entry, name) {
				t.Errorf("an unarmed connection still carries %q", entry)
			}
		}
	}
	if !slices.Contains(session.Env, "HOME=/Users/tester") {
		t.Errorf("environment = %#v, want the inherited HOME", session.Env)
	}
}

// 部分的に埋まった資格情報では武装しない。理由は表示できる文で報告され、接続は
// OpenSSH 自身が尋ねる普通の接続として続く。それは後退ではなく、素の ssh と同じである。
func TestAnIncompleteCredentialFallsBackToAPlainConnection(t *testing.T) {
	tests := map[string]func(*platform.AskpassCredential){
		"an unsupported kind":         func(c *platform.AskpassCredential) { c.Kind = "password" },
		"a helper found through PATH": func(c *platform.AskpassCredential) { c.Helper = "sshc" },
		"no endpoint":                 func(c *platform.AskpassCredential) { c.URL = "" },
		"no identity file":            func(c *platform.AskpassCredential) { c.IdentityFile = "" },
		"no frozen configuration":     func(c *platform.AskpassCredential) { c.SSHConfig = "" },
	}
	for name, damage := range tests {
		t.Run(name, func(t *testing.T) {
			credential := keyPassphraseCredential()
			damage(&credential)
			session, cleanup, err := platform.InteractiveSSH(platform.InteractiveRequest{
				SSH: "/usr/bin/ssh", Alias: "bastion", Credential: credential,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			if session.Armed {
				t.Fatal("an incomplete credential armed the connection")
			}
			if session.Notice == "" {
				t.Fatal("the fallback carried no reason")
			}
			if !slices.Equal(session.Arguments, []string{"--", "bastion"}) {
				t.Fatalf("arguments = %#v", session.Arguments)
			}
			for _, entry := range session.Env {
				if strings.HasPrefix(entry, platform.AskpassTokenVariable+"=") {
					t.Fatalf("the token reached an unarmed connection: %q", entry)
				}
			}
		})
	}
}

// alias と ssh のパスは、組み立てより手前で拒否される。
func TestInteractiveSSHRefusesAnUnsafeAliasOrARelativeProgram(t *testing.T) {
	if _, _, err := platform.InteractiveSSH(platform.InteractiveRequest{
		SSH: "/usr/bin/ssh", Alias: "bastion; rm -rf /",
	}); !errors.Is(err, platform.ErrUnsafeAlias) {
		t.Fatalf("an unsafe alias = %v, want ErrUnsafeAlias", err)
	}
	if _, _, err := platform.InteractiveSSH(platform.InteractiveRequest{
		SSH: "ssh", Alias: "bastion",
	}); !errors.Is(err, platform.ErrInteractiveProgram) {
		t.Fatalf("a relative program = %v, want ErrInteractiveProgram", err)
	}
}

func TestTheFrozenConfigurationIsPrivateAndRemovedAfterUse(t *testing.T) {
	path, cleanup, err := platform.FreezeSSHConfig("Host bastion\n\tHostName example.invalid\n\tUser tester\n")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("temporary config mode = %04o, want 0600", info.Mode().Perm())
	}
	directory, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if directory.Mode().Perm() != 0o700 {
		t.Fatalf("temporary directory mode = %04o, want 0700", directory.Mode().Perm())
	}

	// 後始末は冪等である。呼び出し側は defer と明示の両方で呼ぶ。
	cleanup()
	cleanup()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary config survived cleanup: %v", err)
	}
}
