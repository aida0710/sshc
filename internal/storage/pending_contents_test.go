package storage

import (
	"bytes"
	"errors"
	"os"
	"testing"
)

// interruptSecondWrite は、2つ目のファイルの置き換えで rename を失敗させ、1つ目だけを
// 書き終えた中断した変更を残す。third は新しく作るファイルで、まだ書かれない。
func interruptSecondWrite(t *testing.T) (manager *Manager, first, second, third string) {
	t.Helper()
	workspace := newTestWorkspace(t)
	first = writeWorkspaceFile(t, workspace, "first.conf", "Host first\n", 0o600)
	second = writeWorkspaceFile(t, workspace, "second.conf", "Host second\n", 0o600)
	third = workspace.Root() + string(os.PathSeparator) + "third.conf"
	failure := errors.New("injected rename failure")
	workspace.fileSystem = faultyFileSystem{
		FileSystem: OSFileSystem{},
		failOn: func(operation, path string) error {
			if operation == "rename" && path == second {
				return failure
			}
			return nil
		},
	}
	manager = NewManager(workspace, fixedClock(), bytes.NewReader(bytes.Repeat([]byte{0x5a}, 4096)))
	_, err := manager.Commit(Request{
		Operation: "config.save",
		Changes: []Change{
			{Path: first, Contents: []byte("Host first changed\n"), Precondition: Precondition{Exists: true, Digest: Digest([]byte("Host first\n"))}},
			{Path: second, Contents: []byte("Host second changed\n"), Precondition: Precondition{Exists: true, Digest: Digest([]byte("Host second\n"))}},
			{Path: third, Contents: []byte("Host third\n"), Precondition: Precondition{}},
		},
	})
	if !errors.Is(err, failure) {
		t.Fatalf("commit error = %v, want the injected failure", err)
	}
	workspace.fileSystem = OSFileSystem{}
	return manager, first, second, third
}

func onlyPending(t *testing.T, manager *Manager) Pending {
	t.Helper()
	pending, err := manager.Pending()
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending = %#v, %v; want one interrupted change", pending, err)
	}
	return pending[0]
}

func TestPendingContentsShowTheBeforeAndAfterOfWrittenAndUnwrittenFiles(t *testing.T) {
	manager, first, second, third := interruptSecondWrite(t)

	contents, err := manager.PendingContents(onlyPending(t, manager).ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(contents) != 3 {
		t.Fatalf("contents = %#v, want three files", contents)
	}
	want := []struct {
		path          string
		committed     bool
		before, after string
		beforeExists  bool
	}{
		{first, true, "Host first\n", "Host first changed\n", true},
		{second, false, "Host second\n", "Host second changed\n", true},
		{third, false, "", "Host third\n", false},
	}
	for index, expected := range want {
		got := contents[index]
		if got.Path != expected.path || got.Action != actionWrite || got.Committed != expected.committed {
			t.Fatalf("file %d = %s %s committed=%v, want %s write committed=%v", index, got.Path, got.Action, got.Committed, expected.path, expected.committed)
		}
		if got.Before.Exists != expected.beforeExists || got.Before.Err != nil || string(got.Before.Contents) != expected.before {
			t.Fatalf("%s before = %+v, want %q", got.Path, got.Before, expected.before)
		}
		if !got.After.Exists || got.After.Err != nil || string(got.After.Contents) != expected.after {
			t.Fatalf("%s after = %+v, want %q", got.Path, got.After, expected.after)
		}
	}
}

func TestReadingPendingContentsLeavesTheChangeToCompleteOrRollBack(t *testing.T) {
	manager, first, second, _ := interruptSecondWrite(t)
	identifier := onlyPending(t, manager).ID

	if _, err := manager.PendingContents(identifier); err != nil {
		t.Fatal(err)
	}
	if err := manager.Rollback(identifier); err != nil {
		t.Fatalf("rollback after reading the contents: %v", err)
	}
	for path, want := range map[string]string{first: "Host first\n", second: "Host second\n"} {
		if contents, err := os.ReadFile(path); err != nil || string(contents) != want {
			t.Fatalf("%s = %q, %v; want %q", path, contents, err, want)
		}
	}
}

func TestPendingContentsSayWhyTheAfterSideIsGoneWhenTheStagedFileWasAltered(t *testing.T) {
	manager, _, second, _ := interruptSecondWrite(t)
	identifier := onlyPending(t, manager).ID
	record, _, err := manager.readPendingRecord(identifier)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range record.Entries {
		if entry.Path == second {
			if err := os.WriteFile(entry.Temp, []byte("someone else\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}

	contents, err := manager.PendingContents(identifier)
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(contents[1].After.Err, ErrCannotComplete) {
		t.Fatalf("after of the altered staged file = %+v, want ErrCannotComplete", contents[1].After)
	}
}

func TestPendingContentsRefuseAnUnknownChange(t *testing.T) {
	manager, _ := newTestManager(t)
	if _, err := manager.PendingContents("20261030T131205Z-0f3a"); !errors.Is(err, ErrUnknownTransaction) {
		t.Fatalf("error = %v, want ErrUnknownTransaction", err)
	}
}
