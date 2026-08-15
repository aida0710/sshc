//go:build !windows

package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestUnixPrivateStateContainmentRemainsCaseSensitive(t *testing.T) {
	state := "/home/aida/.ssh/sshc"
	if !privateStateContains(state, state+"/trash/entry") {
		t.Fatal("exact-case descendant was rejected")
	}
	if privateStateContains(state, "/home/aida/.ssh/SSHC/trash/entry") {
		t.Fatal("case-different Unix path was treated as private state")
	}
}

func TestUnixLoadedJournalClaimsRemainCaseSensitive(t *testing.T) {
	claimed := []string{"/home/aida/.ssh/keys/id_work"}
	if journalPathAlreadyClaimed(claimed, "/home/aida/.ssh/keys/id_other") {
		t.Fatal("different Unix path was treated as already claimed")
	}
	if journalPathAlreadyClaimed(claimed, "/home/aida/.ssh/KEYS/ID_WORK") {
		t.Fatal("Unix case alias was treated as the same path")
	}
	if !journalPathAlreadyClaimed(claimed, claimed[0]) {
		t.Fatal("exact Unix path was not treated as already claimed")
	}
}

func TestUnixWritersKeepCaseDistinctClaims(t *testing.T) {
	stop := errors.New("stop before journal mutation")
	for _, test := range []struct {
		name string
		call func(*Manager, *Workspace) error
	}{
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
			workspace := newTestWorkspace(t)
			manager := NewManager(workspace, fixedClock(), pathPolicyErrorReader{err: stop})
			if err := test.call(manager, workspace); !errors.Is(err, stop) {
				t.Fatalf("case-distinct %s = %v, want random stop after claims", test.name, err)
			}
			if _, err := os.Stat(workspace.StateDir()); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("case-distinct %s mutated state: %v", test.name, err)
			}
		})
	}
}

type pathPolicyErrorReader struct{ err error }

func (reader pathPolicyErrorReader) Read([]byte) (int, error) { return 0, reader.err }
