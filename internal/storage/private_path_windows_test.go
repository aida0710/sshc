//go:build windows

package storage

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWindowsPrivateStateContainmentIsCaseInsensitive(t *testing.T) {
	for _, state := range []string{
		filepath.Join(`C:\Users\Aida\.ssh`, "sshc"),
		filepath.Join(`\\Server\Share\Users\Aida\.ssh`, "sshc"),
	} {
		t.Run(state, func(t *testing.T) {
			for name, path := range map[string]string{
				"same directory": strings.ToUpper(state),
				"descendant":     strings.ToUpper(filepath.Join(state, "trash", "entry", "id_work")),
			} {
				t.Run(name, func(t *testing.T) {
					if !privateStateContains(state, path) {
						t.Fatalf("privateStateContains(%q, %q) = false", state, path)
					}
				})
			}
			if privateStateContains(state, strings.ToUpper(state)+"-outside") {
				t.Fatal("case-insensitive prefix without a path boundary was accepted")
			}
		})
	}
}

func TestWindowsWritersRejectCaseAliasDuplicatesBeforeMutation(t *testing.T) {
	for _, test := range []struct {
		name string
		call func(*Manager, *Workspace) error
	}{
		// ルートの綴りはそのまま保ち、その下の要素の大文字小文字だけを変える。
		// Workspace.Contains はどの OS でも大小を区別する文字列比較なので、ルート自体を
		// 綴り替えると、主張リストの検査に届く前に ErrOutsideWorkspace で止まる。
		{
			name: "Commit",
			call: func(manager *Manager, workspace *Workspace) error {
				_, err := manager.Commit(Request{
					Operation: "directory.create",
					Directories: []DirectoryCreate{
						{Path: filepath.Join(workspace.Root(), "Keys")},
						{Path: filepath.Join(workspace.Root(), "keys")},
					},
				})
				return err
			},
		},
		{
			name: "Note",
			call: func(manager *Manager, workspace *Workspace) error {
				path := filepath.Join(workspace.Root(), "config")
				if err := os.WriteFile(path, []byte("Host test\n"), 0o600); err != nil {
					return err
				}
				_, err := manager.Note("config.inspect", []string{path, filepath.Join(workspace.Root(), "CONFIG")})
				return err
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager, workspace := newTestManager(t)
			if err := test.call(manager, workspace); !errors.Is(err, ErrDuplicatePath) {
				t.Fatalf("case-alias %s = %v, want ErrDuplicatePath", test.name, err)
			}
			if _, err := os.Stat(workspace.StateDir()); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("case-alias %s mutated state: %v", test.name, err)
			}
		})
	}
}

func TestWindowsLoadedJournalRejectsCaseAliasPathAndTarget(t *testing.T) {
	root := `C:\Users\Aida\.ssh`
	manager := &Manager{workspace: &Workspace{root: root}}
	digest := Digest([]byte("value"))
	record := journalRecord{
		ID:        validJournalTestID,
		Version:   journalVersion,
		Operation: "key.move",
		Status:    statusStaged,
		StartedAt: time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC),
		Entries: []journalEntry{{
			Action:         actionMove,
			Path:           filepath.Join(root, "Keys", "id_work"),
			Target:         filepath.Join(strings.ToLower(root), "keys", "ID_WORK"),
			HadPrevious:    true,
			Mode:           0o600,
			Digest:         digest,
			PreviousDigest: digest,
		}},
	}
	if err := manager.validateLoadedJournalRecord(record, validJournalTestID+".json", manager.journalDirectory()); !errors.Is(err, ErrInvalidJournal) {
		t.Fatalf("case-alias Path/Target journal = %v, want ErrInvalidJournal", err)
	}
}

// 書き込み先の解決は、**確かめた綴り**を返さなければならない。呼び出し側の
// 綴りをそのまま返すと、検査したのはルートから組み立てた鎖なのに、書くのは
// 別の綴りということになる。
func TestWindowsResolveReturnsTheValidatedRootSpelling(t *testing.T) {
	_, workspace := newTestManager(t)
	if err := workspace.EnsureDirectory(filepath.Join(workspace.Root(), "conf.d")); err != nil {
		t.Fatal(err)
	}
	shouted := filepath.Join(strings.ToUpper(workspace.Root()), "CONF.D", "work.conf")

	resolved, err := workspace.ResolveForWrite(shouted)
	if err != nil {
		t.Fatalf("ResolveForWrite(%q) = %v", shouted, err)
	}
	if !strings.HasPrefix(resolved, workspace.Root()+string(filepath.Separator)) {
		t.Fatalf("resolved = %q, want it under the root spelling %q", resolved, workspace.Root())
	}
}

// ワークスペースのルートそのものは、書き込み先でも作成先でもない。
//
// **素の文字列比較で弾いてはならない。** まわりの包含判断は大小文字を畳むので、
// ルートの別綴りだけがそこをすり抜ける。
func TestWindowsResolveRefusesACaseVariantOfTheRootItself(t *testing.T) {
	_, workspace := newTestManager(t)
	for name, candidate := range map[string]string{
		"exact":        workspace.Root(),
		"case variant": strings.ToUpper(workspace.Root()),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := workspace.ResolveForWrite(candidate); !errors.Is(err, ErrOutsideWorkspace) {
				t.Errorf("ResolveForWrite(%q) = %v, want ErrOutsideWorkspace", candidate, err)
			}
			if _, err := workspace.ResolveDirectory(candidate); !errors.Is(err, ErrOutsideWorkspace) {
				t.Errorf("ResolveDirectory(%q) = %v, want ErrOutsideWorkspace", candidate, err)
			}
		})
	}
}
