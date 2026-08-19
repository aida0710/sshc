package application

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sshc/internal/keys"
	"sshc/internal/storage"
)

// newShortHomeService は、**短い**ホームの上にサービスを作る。
//
// 長さがこの検査の一部である。OpenSSH は鍵の prompt path を `%.100s` で表示し、
// 切り詰められたパスは鍵の同一性を名乗れないので、directKeyPassphraseTarget は
// 100 バイトを超える path をそこで諦める。t.TempDir() の名前は試験名を含むぶん
// 長く、この境界を越える——越えれば、ここにある検査はどれも「対象なし」を
// 正解として通過してしまう。
func newShortHomeService(t *testing.T) (*Service, *storage.Workspace) {
	t.Helper()
	home, err := os.MkdirTemp(shortTempBase(), "sshc-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.EnsureDirectory(workspace.Root()); err != nil {
		t.Fatal(err)
	}
	manager := storage.NewManager(workspace, time.Now, bytes.NewReader(bytes.Repeat([]byte{0x5a}, 4096)))
	return NewService(workspace, manager), workspace
}

// newEligibilityService は、これらの規則が扱う 4 個の状況を宣言するエントリファイル
// と、そのうち 1 個を知っている known_hosts を持つワークスペースを書き出す。
func newEligibilityService(t *testing.T) *Service {
	t.Helper()
	service, workspace := newTestService(t)
	entry := "Host known\n" +
		"\tHostName 203.0.113.10\n" +
		"\n" +
		"Host keyed\n" +
		"\tHostName 198.51.100.7\n" +
		"\tIdentityFile ~/.ssh/keys/id_ed25519\n" +
		"\n" +
		"Host none\n" +
		"\tHostName 198.51.100.9\n" +
		"\tIdentityFile none\n" +
		"\n" +
		"Host nopassword\n" +
		"\tHostName 198.51.100.8\n" +
		"\tPasswordAuthentication no\n" +
		"\n" +
		"Host oddport\n" +
		"\tHostName 203.0.113.20\n" +
		"\tPort 2222\n"
	if err := os.WriteFile(filepath.Join(workspace.Root(), "config"), []byte(entry), 0o600); err != nil {
		t.Fatal(err)
	}
	known := "203.0.113.10 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGZpeHR1cmVrZXlmaXh0dXJla2V5Zml4dHVyZWtl fixture\n" +
		"[203.0.113.20]:2222 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGZpeHR1cmVrZXlmaXh0dXJla2V5Zml4dHVyZWtl fixture\n"
	if err := os.WriteFile(filepath.Join(workspace.Root(), "known_hosts"), []byte(known), 0o600); err != nil {
		t.Fatal(err)
	}
	return service
}

func codesOf(notices []Notice) map[string]bool {
	codes := map[string]bool{}
	for _, notice := range notices {
		codes[notice.Code] = true
	}
	return codes
}

func TestAHostThatRefusesPasswordAuthenticationCannotStoreOne(t *testing.T) {
	// PasswordAuthentication はクライアント側の設定なので、これが off だと
	// クライアントはどれほど良いパスワードでも決して提示しない。保存する
	// ことは、まったく使い道のない秘密をディスクに置くことになる。
	report, err := newEligibilityService(t).PasswordEligibility("nopassword")
	if err != nil {
		t.Fatal(err)
	}
	if report.Storable {
		t.Error("a host that will never be offered a password accepted one")
	}
	if !codesOf(report.Blockers)[BlockerPasswordAuthenticationOff] {
		t.Errorf("blockers = %#v", report.Blockers)
	}
}

func TestAConfiguredDirectKeyBlocksAStoredPassword(t *testing.T) {
	report, err := newEligibilityService(t).PasswordEligibility("keyed")
	if err != nil {
		t.Fatal(err)
	}
	if report.Storable {
		t.Error("a direct key accepted a stored password")
	}
	if !codesOf(report.Blockers)[BlockerIdentityFileConfigured] {
		t.Errorf("blockers = %#v", report.Blockers)
	}
	if codesOf(report.Warnings)[BlockerIdentityFileConfigured] {
		t.Errorf("direct key remained a warning: %#v", report.Warnings)
	}
}

func TestIdentityFileNoneDoesNotBlockAStoredPassword(t *testing.T) {
	report, err := newEligibilityService(t).PasswordEligibility("none")
	if err != nil {
		t.Fatal(err)
	}
	if !report.Storable || codesOf(report.Blockers)[BlockerIdentityFileConfigured] {
		t.Errorf("IdentityFile none reported %#v / %#v", report.Blockers, report.Warnings)
	}
}

func TestDirectKeyPassphraseTargetNamesTheOneWorkspaceKeyOpenSSHWillPromptFor(t *testing.T) {
	service, workspace := newShortHomeService(t)
	entry := "Host keyed\n" +
		"\tIdentityFile ~/.ssh/id_ed25519_server\n" +
		"Host complex\n" +
		"\tIdentityFile ~/.ssh/id_first\n" +
		"\tIdentityFile ~/.ssh/id_second\n" +
		"Host inherited\n" +
		"\tHostName inherited.example\n" +
		"Host duplicate\n" +
		"\tIdentityFile ~/.ssh/id_duplicate\n" +
		"Host duplicate\n" +
		"\tIdentityFile ~/.ssh/id_duplicate_second\n" +
		"Host *\n" +
		"\tServerAliveInterval 30\n"
	if err := os.WriteFile(filepath.Join(workspace.Root(), "config"), []byte(entry), 0o600); err != nil {
		t.Fatal(err)
	}

	target, ok, err := service.directKeyPassphraseTarget("keyed")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("the direct workspace key was not resolved")
	}
	if target.RelativePath != "id_ed25519_server" {
		t.Fatalf("relative path = %q", target.RelativePath)
	}

	for _, alias := range []string{"complex", "inherited", "duplicate"} {
		if _, ok, err := service.directKeyPassphraseTarget(alias); err != nil || ok {
			t.Errorf("DirectKeyPassphraseTarget(%q) = ok %v, err %v; want no target", alias, ok, err)
		}
	}
}

func TestDirectKeyPassphraseTargetRefusesInheritedKeysAndExecutableConfiguration(t *testing.T) {
	for name, entry := range map[string]string{
		"inherited second key": "Host keyed\n\tIdentityFile ~/.ssh/id_direct\nHost *\n\tIdentityFile ~/.ssh/id_global\n",
		"proxy command":        "Host keyed\n\tIdentityFile ~/.ssh/id_direct\n\tProxyCommand helper %h %p\n",
		"proxy jump":           "Host keyed\n\tIdentityFile ~/.ssh/id_direct\n\tProxyJump bastion\n",
		"match block":          "Host keyed\n\tIdentityFile ~/.ssh/id_direct\nMatch all\n\tUser ops\n",
		"xauth helper":         "Host keyed\n\tIdentityFile ~/.ssh/id_direct\n\tForwardX11 yes\n\tXAuthLocation ~/.ssh/read-token\n",
		"canonical second pass": "Host keyed\n\tCanonicalizeHostname yes\n\tCanonicalDomains example.test\n\tIdentityFile ~/.ssh/id_direct\n" +
			"Host keyed.example.test\n\tIdentityFile ~/.ssh/id_after_canonicalisation\n",
	} {
		t.Run(name, func(t *testing.T) {
			service, workspace := newTestService(t)
			if err := os.WriteFile(filepath.Join(workspace.Root(), "config"), []byte(entry), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, ok, err := service.directKeyPassphraseTarget("keyed"); err != nil || ok {
				t.Fatalf("target = ok %v, err %v; want conservative refusal", ok, err)
			}
		})
	}
}

func TestDirectKeyPassphraseTargetRequiresACurrentEncryptedPrivateKey(t *testing.T) {
	service, workspace := newShortHomeService(t)
	if err := os.WriteFile(filepath.Join(workspace.Root(), "config"),
		[]byte("Host keyed\n\tIdentityFile ~/.ssh/id_ed25519_server\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := DirectKeyPassphraseTarget{RelativePath: "id_ed25519_server"}
	for _, item := range []keys.Item{
		{RelativePath: target.RelativePath, Kind: keys.KindPrivateKey, Encrypted: false},
		{RelativePath: target.RelativePath, Kind: keys.KindOther, Encrypted: true},
	} {
		inventory := func() (*keys.Inventory, error) { return &keys.Inventory{Items: []keys.Item{item}}, nil }
		if _, ok, err := service.DirectKeyPassphraseTarget("keyed", inventory); err != nil || ok {
			t.Errorf("item %+v = ok %v, err %v; want no target", item, ok, err)
		}
	}
	encrypted := func() (*keys.Inventory, error) {
		return &keys.Inventory{Items: []keys.Item{{
			RelativePath: target.RelativePath, Kind: keys.KindPrivateKey, Encrypted: true, ContentDigest: "key-digest",
		}}}, nil
	}
	if got, ok, err := service.DirectKeyPassphraseTarget("keyed", encrypted); err != nil || !ok ||
		got.RelativePath != target.RelativePath {
		t.Fatalf("encrypted key = %+v, ok %v, err %v", got, ok, err)
	}
}

func TestAnUnknownHostKeyIsReportedBecauseTheHelperWillNotAnswerThatQuestion(t *testing.T) {
	// askpass を強制すると、ホスト鍵の問いも helper へ回されるが、
	// helper はそれを拒否する。そのため未検証のホストへの最初の接続は
	// そのプロンプトでパスワードが使われないまま止まる。それをここで
	// 言明することが、壊れているように見える機能と自ら説明する機能との
	// 違いになる。
	report, err := newEligibilityService(t).PasswordEligibility("keyed")
	if err != nil {
		t.Fatal(err)
	}
	if !codesOf(report.Warnings)[WarnHostKeyUnknown] {
		t.Errorf("an unverified host was not reported: %#v", report.Warnings)
	}

	known, err := newEligibilityService(t).PasswordEligibility("known")
	if err != nil {
		t.Fatal(err)
	}
	if codesOf(known.Warnings)[WarnHostKeyUnknown] {
		t.Errorf("a host already in known_hosts was reported as unknown: %#v", known.Warnings)
	}
}

func TestANonDefaultPortIsLookedUpInTheFormKnownHostsUses(t *testing.T) {
	// known_hosts はデフォルト以外のポートを[host]:port として書く。素の
	// ホストだけを調べると、そのようなホストすべてを未検証として報告して
	// しまい、常に出ている warning は誰も読まない warning になる。
	report, err := newEligibilityService(t).PasswordEligibility("oddport")
	if err != nil {
		t.Fatal(err)
	}
	if report.Port != "2222" {
		t.Errorf("port = %q", report.Port)
	}
	if codesOf(report.Warnings)[WarnHostKeyUnknown] {
		t.Errorf("a host known at its own port was reported as unknown: %#v", report.Warnings)
	}
}

func TestAPatternIsNotAHostAndCannotHoldAPassword(t *testing.T) {
	report, err := newEligibilityService(t).PasswordEligibility("*")
	if err != nil {
		t.Fatal(err)
	}
	if report.Storable {
		t.Error("a pattern accepted a password")
	}
	if !codesOf(report.Blockers)[BlockerAliasNotSimple] {
		t.Errorf("blockers = %#v", report.Blockers)
	}
}

func TestAnOrdinaryVerifiedHostHasNothingToSay(t *testing.T) {
	// すべてのホストに出る warning は雑音であり、
	// 雑音こそが本当の warning が無視される原因である。
	report, err := newEligibilityService(t).PasswordEligibility("known")
	if err != nil {
		t.Fatal(err)
	}
	if !report.Storable || len(report.Blockers) != 0 || len(report.Warnings) != 0 {
		t.Errorf("an ordinary host reported %#v / %#v", report.Blockers, report.Warnings)
	}
	if report.HostName != "203.0.113.10" {
		t.Errorf("hostName = %q", report.HostName)
	}
}

// **Match の下に書かれた設定も、この報告に載らなければならない。**
//
// ここは長いあいだ effective.Project を使っていた。あの射影は Match ブロックの値を
// 決して採らず、「接続中にしか分からない条件がある」という印を complexity として
// 脇に置くだけである。ところがこの報告は complexity を一度も読まなかったので、
// 下の設定に対して「保存してよい」と答えていた——そして保存されたパスワードは、
// PasswordAuthentication no のせいで一度も提示されない。
//
// Match host は接続中の状態を要さない。Resolve はこれを評価するので、答えが出る。
func TestPasswordAuthenticationOffInsideAMatchBlockStillBlocks(t *testing.T) {
	service, workspace := newTestService(t)
	entry := "Host guarded\n" +
		"\tHostName 198.51.100.30\n" +
		"\n" +
		"Match host guarded\n" +
		"\tPasswordAuthentication no\n"
	if err := os.WriteFile(filepath.Join(workspace.Root(), "config"), []byte(entry), 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := service.PasswordEligibility("guarded")
	if err != nil {
		t.Fatal(err)
	}
	if report.Storable {
		t.Error("a host whose Match block refuses password authentication accepted one")
	}
	if !codesOf(report.Blockers)[BlockerPasswordAuthenticationOff] {
		t.Errorf("blockers = %#v", report.Blockers)
	}
}

// Match の下の HostName も同じように効く。**known_hosts を引く相手が変わる。**
func TestHostNameInsideAMatchBlockReachesTheReport(t *testing.T) {
	service, workspace := newTestService(t)
	entry := "Host shifting\n" +
		"\tPort 22\n" +
		"\n" +
		"Match host shifting\n" +
		"\tHostName 198.51.100.31\n"
	if err := os.WriteFile(filepath.Join(workspace.Root(), "config"), []byte(entry), 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := service.PasswordEligibility("shifting")
	if err != nil {
		t.Fatal(err)
	}
	if report.HostName != "198.51.100.31" {
		t.Errorf("HostName = %q, want the value the Match block sets", report.HostName)
	}
}

// **誰も書いていない Port を、書かれた値として報告しない。**
//
// 解決器は hostname・user・port の 3 つに既定値を持つ。値の側（Resolution.Values）
// から Port を読むと、設定に一行も無いのに 22 が載る——そして known_hosts は
// `[host]:22` ではなく `host` の綴りで書かれるので、引く相手が変わる。
func TestAnUnwrittenPortIsNotReported(t *testing.T) {
	service, workspace := newTestService(t)
	entry := "Host plain\n\tHostName 198.51.100.32\n"
	if err := os.WriteFile(filepath.Join(workspace.Root(), "config"), []byte(entry), 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := service.PasswordEligibility("plain")
	if err != nil {
		t.Fatal(err)
	}
	if report.Port != "" {
		t.Errorf("Port = %q, want empty because the configuration does not set one", report.Port)
	}
}
