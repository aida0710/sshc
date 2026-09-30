package storage

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

var errInjectedStagingFault = errors.New("injected staging fault")

// temporaryFilesIn は、directory に残っている一時ファイルの名前を返す。
func temporaryFilesIn(t *testing.T, directory string) []string {
	t.Helper()
	children, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, child := range children {
		if IsTemporaryName(child.Name()) {
			names = append(names, child.Name())
		}
	}
	return names
}

// twoWriteRequest は、ワークスペースの直下の 2 ファイルを書き換える要求を作る。
func twoWriteRequest(t *testing.T, workspace *Workspace) Request {
	t.Helper()
	first := writeWorkspaceFile(t, workspace, "first.conf", "first before\n", 0o600)
	second := writeWorkspaceFile(t, workspace, "second.conf", "second before\n", 0o600)
	return Request{
		Operation: "connection.update",
		Changes: []Change{
			{Path: first, Contents: []byte("first after\n"), Precondition: Precondition{Exists: true, Digest: Digest([]byte("first before\n"))}},
			{Path: second, Contents: []byte("second after\n"), Precondition: Precondition{Exists: true, Digest: Digest([]byte("second before\n"))}},
		},
	}
}

// failNth は、operation が directory の中で n 回目に起きたときだけ失敗させる。
func failNth(operation, directory string, n int) func(string, string) error {
	count := 0
	return func(observed, path string) error {
		if observed != operation || (path != directory && filepath.Dir(path) != directory) {
			return nil
		}
		count++
		if count == n {
			return errInjectedStagingFault
		}
		return nil
	}
}

// ステージの途中で失敗した非 atomic の取引は、対象の隣に一時ファイルを残さない。
// 残ると、鍵の一覧や Include の glob に紛れ込み、Rollback でも消えない。
func TestAFailureWhileStagingLeavesNoTemporaryFileBesideTheTargets(t *testing.T) {
	cases := map[string]func(workspace *Workspace) func(string, string) error{
		"the second staged file cannot be written": func(workspace *Workspace) func(string, string) error {
			return failNth("writeTemp", workspace.Root(), 2)
		},
		"the staged record cannot be written": func(workspace *Workspace) func(string, string) error {
			return failNth("rename", filepath.Join(workspace.StateDir(), journalDirectoryName), 2)
		},
	}
	for name, fault := range cases {
		t.Run(name, func(t *testing.T) {
			workspace := newTestWorkspace(t)
			request := twoWriteRequest(t, workspace)
			workspace.fileSystem = faultyFileSystem{FileSystem: OSFileSystem{}, failOn: fault(workspace)}

			result, err := NewManager(workspace, fixedClock(), bytes.NewReader(bytes.Repeat([]byte{0x7e}, 4096))).Commit(request)
			if !errors.Is(err, errInjectedStagingFault) {
				t.Fatalf("Commit = %v, want the injected fault", err)
			}
			if leftovers := temporaryFilesIn(t, workspace.Root()); len(leftovers) != 0 {
				t.Fatalf("staged files were left beside the targets: %v", leftovers)
			}

			workspace.fileSystem = OSFileSystem{}
			if err := restartedManager(t, workspace).Rollback(result.ID); err != nil {
				t.Fatalf("Rollback = %v", err)
			}
			if leftovers := temporaryFilesIn(t, workspace.Root()); len(leftovers) != 0 {
				t.Fatalf("staged files survived the rollback: %v", leftovers)
			}
		})
	}
}

// ステージの途中でプロセスが落ちると、一時ファイルの名前は記録に載らない。
// それでも Rollback は、記録の ID で始まる一時ファイルを消す。
func TestRollbackRemovesTemporaryFilesThatTheRecordNeverNamed(t *testing.T) {
	workspace := newTestWorkspace(t)
	request := twoWriteRequest(t, workspace)
	stageFailure := failNth("writeTemp", workspace.Root(), 2)
	workspace.fileSystem = faultyFileSystem{FileSystem: OSFileSystem{}, failOn: func(operation, path string) error {
		// 一時ファイルを消せない状態にして、落ちたプロセスが残したのと同じ残骸を作る。
		if operation == "remove" && filepath.Dir(path) == workspace.Root() && IsTemporaryName(filepath.Base(path)) {
			return errInjectedStagingFault
		}
		return stageFailure(operation, path)
	}}
	result, err := NewManager(workspace, fixedClock(), bytes.NewReader(bytes.Repeat([]byte{0x7f}, 4096))).Commit(request)
	if !errors.Is(err, errInjectedStagingFault) {
		t.Fatalf("Commit = %v, want the injected fault", err)
	}
	if leftovers := temporaryFilesIn(t, workspace.Root()); len(leftovers) == 0 {
		t.Fatal("the fixture left no staged file to recover")
	}

	workspace.fileSystem = OSFileSystem{}
	if err := restartedManager(t, workspace).Rollback(result.ID); err != nil {
		t.Fatalf("Rollback = %v", err)
	}
	if leftovers := temporaryFilesIn(t, workspace.Root()); len(leftovers) != 0 {
		t.Fatalf("staged files survived the rollback: %v", leftovers)
	}
}
