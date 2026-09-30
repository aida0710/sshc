package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// バックアップを捨てる commit（マスターパスワード変更）は、暗号化済み文書を控えごと
// 置き換える書き込みしか巻き戻せない。ほかの操作は、ファイルに触れる前に断る。
func TestDiscardBackupsCommitRefusesEverythingButBackedUpWrites(t *testing.T) {
	for _, test := range []struct {
		name    string
		request func(root, target string) Request
	}{
		{
			name: "directory creation",
			request: func(root, target string) Request {
				return Request{Directories: []DirectoryCreate{{Path: filepath.Join(root, "created")}}}
			},
		},
		{
			name: "move",
			request: func(root, target string) Request {
				return Request{Moves: []Move{{From: target, To: filepath.Join(root, "moved")}}}
			},
		},
		{
			name: "removal",
			request: func(root, target string) Request {
				return Request{Removals: []Removal{{Path: target, Backup: true}}}
			},
		},
		{
			name: "directory removal",
			request: func(root, target string) Request {
				return Request{RemoveDirectories: []DirectoryRemoval{{Path: filepath.Join(root, "empty")}}}
			},
		},
		{
			name: "change without backup",
			request: func(root, target string) Request {
				return Request{Changes: []Change{{Path: target, Contents: []byte("after\n"), SkipBackup: true}}}
			},
		},
		{
			name: "final change without backup",
			request: func(root, target string) Request {
				return Request{FinalChanges: []Change{{Path: target, Contents: []byte("after\n"), SkipBackup: true}}}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, commit := range []struct {
				name string
				run  func(manager *Manager, request Request) (published bool, err error)
			}{
				{
					name: "CommitAtomicDiscardBackups",
					run: func(manager *Manager, request Request) (bool, error) {
						_, err := manager.CommitAtomicDiscardBackups(request)
						return false, err
					},
				},
				{
					name: "CommitAtomicDiscardBackupsAndPublish",
					run: func(manager *Manager, request Request) (bool, error) {
						published := false
						_, err := manager.CommitAtomicDiscardBackupsAndPublish(
							func() (Request, error) { return request, nil },
							func() { published = true },
						)
						return published, err
					},
				},
			} {
				t.Run(commit.name, func(t *testing.T) {
					manager, workspace := newTestManager(t)
					target := writeWorkspaceFile(t, workspace, "vault", "before\n", 0o600)
					request := test.request(workspace.Root(), target)
					request.Operation = "secret.rekey"

					published, err := commit.run(manager, request)
					if !errors.Is(err, ErrAtomicWriteOnly) {
						t.Fatalf("%s = %v, want ErrAtomicWriteOnly", commit.name, err)
					}
					if published {
						t.Fatal("a refused request published its generation")
					}
					if contents, err := os.ReadFile(target); err != nil || string(contents) != "before\n" {
						t.Fatalf("target after refused request = %q, %v", contents, err)
					}
				})
			}
		})
	}
}
