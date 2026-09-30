package application

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"sshc/internal/keys"
)

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
		"\tPort 2222\n" +
		"\n" +
		"Host plainonly\n" +
		"\tHostName 203.0.113.10\n" +
		"\tPort 2222\n" +
		"\n" +
		"Host bykeyalias\n" +
		"\tHostName 203.0.113.30\n" +
		"\tUserKnownHostsFile ~/.ssh/known_hosts.d/%k\n" +
		"\n" +
		"Host unsetvariable\n" +
		"\tHostName 203.0.113.10\n" +
		"\tUserKnownHostsFile ~/.ssh/${SSHC_TEST_VARIABLE_THAT_IS_NEVER_SET}\n"
	if err := os.WriteFile(filepath.Join(workspace.Root(), "config"), []byte(entry), 0o600); err != nil {
		t.Fatal(err)
	}
	known := "203.0.113.10 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGZpeHR1cmVrZXlmaXh0dXJla2V5Zml4dHVyZWtl fixture\n" +
		"[203.0.113.20]:2222 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGZpeHR1cmVrZXlmaXh0dXJla2V5Zml4dHVyZWtl fixture\n"
	if err := os.WriteFile(filepath.Join(workspace.Root(), "known_hosts"), []byte(known), 0o600); err != nil {
		t.Fatal(err)
	}
	perAlias := filepath.Join(workspace.Root(), "known_hosts.d")
	if err := os.MkdirAll(perAlias, 0o700); err != nil {
		t.Fatal(err)
	}
	byKeyAlias := "203.0.113.30 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGZpeHR1cmVrZXlmaXh0dXJla2V5Zml4dHVyZWtl fixture\n"
	if err := os.WriteFile(filepath.Join(perAlias, "bykeyalias"), []byte(byKeyAlias), 0o600); err != nil {
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

func TestWorkspaceKeysNamesEveryKeyTheConnectionCanUse(t *testing.T) {
	service, workspace := newTestService(t)
	entry := "Host keyed\n" +
		"\tIdentityFile ~/.ssh/id_ed25519_server\n" +
		"Host complex\n" +
		"\tIdentityFile ~/.ssh/id_first\n" +
		"\tIdentityFile ~/.ssh/id_second\n" +
		"Host inherited\n" +
		"\tHostName inherited.example\n" +
		"Host outside\n" +
		"\tIdentityFile /etc/ssh/elsewhere\n"
	if err := os.WriteFile(filepath.Join(workspace.Root(), "config"), []byte(entry), 0o600); err != nil {
		t.Fatal(err)
	}

	for alias, want := range map[string][]string{
		"keyed":     {"id_ed25519_server"},
		"complex":   {"id_first", "id_second"},
		"inherited": {},
		"outside":   {},
	} {
		got, err := service.WorkspaceKeys(alias)
		if err != nil {
			t.Fatalf("WorkspaceKeys(%q) = %v", alias, err)
		}
		if !slices.Equal(got, want) {
			t.Errorf("WorkspaceKeys(%q) = %#v, want %#v", alias, got, want)
		}
	}
}

func TestWorkspaceKeysNoLongerHidesBehindDirectivesTheClientNeverRuns(t *testing.T) {
	for name, entry := range map[string]string{
		"inherited second key": "Host keyed\n\tIdentityFile ~/.ssh/id_direct\nHost *\n\tIdentityFile ~/.ssh/id_global\n",
		"proxy command":        "Host keyed\n\tIdentityFile ~/.ssh/id_direct\n\tProxyCommand helper %h %p\n",
		"proxy jump":           "Host keyed\n\tIdentityFile ~/.ssh/id_direct\n\tProxyJump bastion\n",
		"match block":          "Host keyed\n\tIdentityFile ~/.ssh/id_direct\nMatch all\n\tUser ops\n",
		"xauth helper":         "Host keyed\n\tIdentityFile ~/.ssh/id_direct\n\tForwardX11 yes\n\tXAuthLocation ~/.ssh/read-token\n",
	} {
		t.Run(name, func(t *testing.T) {
			service, workspace := newTestService(t)
			if err := os.WriteFile(filepath.Join(workspace.Root(), "config"), []byte(entry), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := service.WorkspaceKeys("keyed")
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(got, "id_direct") {
				t.Errorf("WorkspaceKeys = %#v, want it to contain id_direct", got)
			}
		})
	}
}

func TestWorkspaceKeysStaysSilentWhenTheConfigurationCannotBeResolved(t *testing.T) {
	service, workspace := newTestService(t)
	entry := "Host keyed\n\tCanonicalizeHostname yes\n\tCanonicalDomains example.test\n" +
		"\tIdentityFile ~/.ssh/id_direct\n"
	if err := os.WriteFile(filepath.Join(workspace.Root(), "config"), []byte(entry), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := service.WorkspaceKeys("keyed")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("WorkspaceKeys = %#v, want nothing while the answer depends on DNS", got)
	}
}

func TestUnlockableWorkspaceKeysRequiresACurrentEncryptedPrivateKey(t *testing.T) {
	service, workspace := newTestService(t)
	if err := os.WriteFile(filepath.Join(workspace.Root(), "config"),
		[]byte("Host keyed\n\tIdentityFile ~/.ssh/id_ed25519_server\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	const relative = "id_ed25519_server"
	for _, item := range []keys.Item{
		{RelativePath: relative, Kind: keys.KindPrivateKey, Encrypted: false},
		{RelativePath: relative, Kind: keys.KindOther, Encrypted: true},
	} {
		inventory := func() (*keys.Inventory, error) { return &keys.Inventory{Items: []keys.Item{item}}, nil }
		got, err := service.UnlockableWorkspaceKeys("keyed", inventory)
		if err != nil || len(got) != 0 {
			t.Errorf("item %+v = %#v, err %v; want nothing", item, got, err)
		}
	}
	encrypted := func() (*keys.Inventory, error) {
		return &keys.Inventory{Items: []keys.Item{{
			RelativePath: relative, Kind: keys.KindPrivateKey, Encrypted: true, ContentDigest: "key-digest",
		}}}, nil
	}
	got, err := service.UnlockableWorkspaceKeys("keyed", encrypted)
	if err != nil || !slices.Equal(got, []string{relative}) {
		t.Fatalf("encrypted key = %#v, err %v", got, err)
	}
}

func TestAnUnknownHostKeyIsReportedBecauseTheHelperWillNotAnswerThatQuestion(t *testing.T) {
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

// OpenSSH 8.4 以降が示す `UserKnownHostsFile ~/.ssh/known_hosts.d/%k` は、%k を
// alias に展開したファイルで照合する。展開できない値があっても、パスワードを
// 保存できるかの判断と認証の束縛の計算は止めない。接続はホスト鍵の照合で断る。
func TestUserKnownHostsFileTokensAreExpandedAndAnUnexpandableOneStopsOnlyTheConnection(t *testing.T) {
	service := newEligibilityService(t)
	report, err := service.PasswordEligibility("bykeyalias")
	if err != nil {
		t.Fatal(err)
	}
	if codesOf(report.Warnings)[WarnHostKeyUnknown] {
		t.Errorf("a host in known_hosts.d/%%k was reported as unknown: %#v", report.Warnings)
	}

	unset, err := service.PasswordEligibility("unsetvariable")
	if err != nil {
		t.Fatalf("PasswordEligibility = %v, want a report", err)
	}
	if !codesOf(unset.Warnings)[WarnHostKeyUnknown] {
		t.Errorf("a host whose known_hosts cannot be named was not reported: %#v", unset.Warnings)
	}
	graph, err := service.resolve()
	if err != nil {
		t.Fatal(err)
	}
	if binding, err := service.passwordBindingForGraph(graph, "unsetvariable"); err != nil || binding == "" {
		t.Errorf("passwordBindingForGraph = %q, %v; want a binding", binding, err)
	}
}

func TestANonDefaultPortIsLookedUpInTheFormKnownHostsUses(t *testing.T) {
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

// 2222 番の接続は [host]:2222 の行だけと照合する。22 番の素の行は別の sshd の
// 記録なので、それしか無ければ警告する。警告しないと、パスワードを保存したあとの
// 初回接続が未知のホストとして失敗する。
func TestANonDefaultPortIsNotKnownThroughThePlainForm(t *testing.T) {
	report, err := newEligibilityService(t).PasswordEligibility("plainonly")
	if err != nil {
		t.Fatal(err)
	}
	if !codesOf(report.Warnings)[WarnHostKeyUnknown] {
		t.Errorf("a host known only at port 22 was not reported: %#v", report.Warnings)
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

func TestPasswordAuthenticationOffInsideAMatchBlockStillBlocks(t *testing.T) {
	service, workspace := newTestService(t)
	// `Match host` compares the resolved HostName, so the block names the
	// address rather than the alias (`Match originalhost guarded` would also do).
	entry := "Host guarded\n" +
		"\tHostName 198.51.100.30\n" +
		"\n" +
		"Match host 198.51.100.30\n" +
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
