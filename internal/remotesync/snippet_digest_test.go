package remotesync_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"sshc/internal/remotesync"
	"sshc/internal/secret"
	"sshc/internal/snippets"
)

// withVaultAndSnippets は、withVault に加えて、本番の配線（internal/app/services.go）
// と同じく vault の鍵で封じたスニペットの文書を同期へ繋ぐ。
func withVaultAndSnippets(t *testing.T, machine *installation, master string) (*secret.Service, *snippets.Store) {
	t.Helper()
	secrets := withVault(t, machine, master)
	store := snippets.NewStore(machine.workspace, snippets.Protection{
		Seal: secrets.SealDocument,
		Open: func(contents []byte) ([]byte, error) {
			plaintext, err := secrets.OpenDocument(contents)
			if errors.Is(err, secret.ErrNotAVault) {
				return nil, snippets.ErrNotEncrypted
			}
			return plaintext, err
		},
		WithMutation: func(mutation func() error) error {
			return secrets.WithStableSnapshot(func() error {
				return machine.manager.WithSnapshot(mutation)
			})
		},
	})
	if err := secrets.RegisterProtectedDocument(secret.ProtectedDocument{
		Path: store.Path(), Validate: store.ValidateDocument,
	}); err != nil {
		t.Fatal(err)
	}
	machine.replaceIntegrations(t, func(hooks *remotesync.IntegrationHooks) {
		useVault(hooks, secrets)
		hooks.OpenSnippets = store.TravelDocument
		hooks.SealSnippets = store.AdoptTravelDocument
	})
	secrets.SetTravelDigestRekey(machine.service.RekeyTravelDigest)
	return secrets, store
}

func saveOneSnippet(t *testing.T, store *snippets.Store) {
	t.Helper()
	moment := time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC)
	if err := store.Save(snippets.Library{Snippets: []snippets.Snippet{{
		ID: "0123456789abcdef0123456789abcdef", Name: "Check", Command: "uptime",
		CreatedAt: moment, UpdatedAt: moment,
	}}, Startup: []snippets.Startup{}}); err != nil {
		t.Fatal(err)
	}
}

// pushedSnippetState は、スニペットを 1 件持つマシンから push し、スニペットの文書の
// 素の digest と state のファイルのパスを返す。
func pushedSnippetState(t *testing.T) (installation, *secret.Service, string, string) {
	t.Helper()
	machine := newInstallation(t, &fakeBucket{}, map[string]string{"config": "Host bastion\n"})
	secrets, store := withVaultAndSnippets(t, &machine, "the first machine's master")
	saveOneSnippet(t, store)
	if _, err := machine.service.PushUsing(context.Background(), keyOf(syncPassphrase), ""); err != nil {
		t.Fatalf("Push = %v", err)
	}
	document, err := store.TravelDocument()
	if err != nil || document == nil {
		t.Fatalf("TravelDocument = %q, %v", document, err)
	}
	statePath := filepath.Join(machine.workspace.Root(), filepath.FromSlash(remotesync.StatePath))
	return machine, secrets, remotesync.Digest(document), statePath
}

func assertNothingToPush(t *testing.T, machine installation, when string) {
	t.Helper()
	draft, err := machine.service.PushDraft()
	if err != nil {
		t.Fatal(err)
	}
	if draft.Added+draft.Modified+draft.Removed != 0 {
		t.Fatalf("%s, the unchanged snippets are reported as a change: %+v", when, draft)
	}
}

// スニペットの文書は、起動スニペットの secret 入力の値を含む。vault 文書と同じく、
// sync-state.json には鍵付きの digest だけを残し、マスターパスワードを変えても前回の
// 同期との比較は変わらない。
func TestTheSyncStateKeepsOnlyAKeyedDigestOfTheSnippetDocument(t *testing.T) {
	machine, secrets, plain, statePath := pushedSnippetState(t)
	state, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(state, []byte(plain)) {
		t.Fatalf("the sync state holds the plain digest of the snippet document:\n%s", state)
	}
	assertNothingToPush(t, machine, "after the push")

	if err := secrets.ChangeMasterPassword(context.Background(), "the first machine's master", "the first machine's new master"); err != nil {
		t.Fatalf("ChangeMasterPassword = %v", err)
	}
	assertNothingToPush(t, machine, "after changing the master password")
}

// 鍵付きにする前の版は、スニペットの文書の素の SHA-256 を state に書いていた。移行は
// それを今の鍵の値へ書き換え、前回の同期との比較は変えない。
func TestMigratingTheTravelDigestReplacesAPlainSnippetDigestLeftByAnEarlierRelease(t *testing.T) {
	machine, _, plain, statePath := pushedSnippetState(t)
	state, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	// 鍵付きの値は vault 文書とスニペットの文書の 2 つ。どちらも素の値へ戻すと、
	// 前の版が書いた state と同じ形になる。スニペットの値を確かめられるよう、
	// vault 文書の値は鍵付きのまま残す。
	keyed := regexp.MustCompile(`("path": "` + regexp.QuoteMeta(remotesync.SnippetsPath) + `",\s*"sha256": )"hmac-sha256:[0-9a-f]+"`)
	earlier := keyed.ReplaceAll(state, []byte(`${1}"`+plain+`"`))
	if bytes.Equal(earlier, state) {
		t.Fatalf("the sync state has no keyed snippet digest to replace:\n%s", state)
	}
	if err := os.WriteFile(statePath, earlier, 0o600); err != nil {
		t.Fatal(err)
	}
	assertNothingToPush(t, machine, "before the migration")

	if err := machine.service.MigrateTravelDigest(); err != nil {
		t.Fatalf("MigrateTravelDigest = %v", err)
	}
	migrated, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(migrated, []byte(plain)) {
		t.Fatalf("the plain snippet digest was not replaced by a keyed one:\n%s", migrated)
	}
	assertNothingToPush(t, machine, "after the migration")
}
