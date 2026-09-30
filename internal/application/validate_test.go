package application

import (
	"crypto/rand"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"sshc/internal/knownhosts"
	"sshc/internal/storage"
)

type mapLoader map[string][]byte

func (loader mapLoader) ReadFile(name string) ([]byte, error) {
	contents, ok := loader[name]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return contents, nil
}

func (loader mapLoader) Glob(pattern string) ([]string, error) {
	var matches []string
	for name := range loader {
		matched, err := filepath.Match(pattern, name)
		if err != nil {
			return nil, err
		}
		if matched {
			matches = append(matches, name)
		}
	}
	sort.Strings(matches)
	return matches, nil
}

func TestOverlayForDescribesWhatArrivesAndWhatLeaves(t *testing.T) {
	pending, gone := overlayFor(storage.Request{
		Changes:  []storage.Change{{Path: "conf.d/20-new.conf", Contents: []byte("Host new\n")}},
		Moves:    []storage.Move{{From: "conf.d/10-old.conf", To: "connections/work/10-old.conf"}},
		Removals: []storage.Removal{{Path: "conf.d/30-dead.conf"}},
	})

	if string(pending[filepath.FromSlash("conf.d/20-new.conf")]) != "Host new\n" {
		t.Fatalf("pending = %#v, want the written contents", pending)
	}
	if !gone[filepath.FromSlash("conf.d/10-old.conf")] {
		t.Errorf("a move's source is not marked gone")
	}
	if !gone[filepath.FromSlash("conf.d/30-dead.conf")] {
		t.Errorf("a removal is not marked gone")
	}
	if gone[filepath.FromSlash("connections/work/10-old.conf")] {
		t.Errorf("a move's destination must not be marked gone")
	}
}

func TestOverlayLoaderHidesAMovedSourceFromReadsAndGlobs(t *testing.T) {
	moved := filepath.Join(testRoot, "conf.d", "10-old.conf")
	kept := filepath.Join(testRoot, "conf.d", "keep.conf")
	destination := filepath.Join(testRoot, "connections", "work", "10-old.conf")
	base := mapLoader{
		moved:       []byte("Host nas\n"),
		kept:        []byte("Host keep\n"),
		destination: []byte("Host nas\n"),
	}
	loader := overlayLoader{
		base:    base,
		pending: map[string][]byte{destination: []byte("Host nas\n")},
		gone:    map[string]bool{moved: true},
	}

	if _, err := loader.ReadFile(moved); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("reading a moved source error = %v, want fs.ErrNotExist", err)
	}
	if _, err := loader.ReadFile(kept); err != nil {
		t.Fatalf("reading an untouched file error = %v", err)
	}

	matches, err := loader.Glob(filepath.Join(testRoot, "conf.d", "*.conf"))
	if err != nil {
		t.Fatalf("Glob error = %v", err)
	}
	if len(matches) != 1 || matches[0] != kept {
		t.Fatalf("Glob = %v, want only the untouched file", matches)
	}
}

func TestOverlayLoaderPrefersPendingContentsOverARemoval(t *testing.T) {
	entry := filepath.Join(testRoot, "config")
	loader := overlayLoader{
		base:    mapLoader{},
		pending: map[string][]byte{entry: []byte("Host rewritten\n")},
		gone:    map[string]bool{entry: true},
	}

	contents, err := loader.ReadFile(entry)
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}
	if string(contents) != "Host rewritten\n" {
		t.Fatalf("contents = %q, want the pending contents", contents)
	}
}

func TestValidateLeavesApplicationStateAlone(t *testing.T) {
	service, workspace := newTestService(t)
	if err := workspace.EnsureDirectory(workspace.StateDir()); err != nil {
		t.Fatal(err)
	}

	ciphertext := []byte("\x91\x2f\"\x00\xd4 not configuration at all\n")

	if _, err := service.manager.Commit(storage.Request{
		Operation: "secret.vault",
		Changes: []storage.Change{{
			Path:       filepath.Join(workspace.StateDir(), "secrets"),
			Contents:   ciphertext,
			SkipBackup: true,
		}},
	}); err != nil {
		t.Fatalf("a file under sshc/ was validated as configuration: %v", err)
	}

	if _, err := service.manager.Commit(storage.Request{
		Operation: "config.file_raw",
		Changes: []storage.Change{{
			Path:     filepath.Join(workspace.Root(), "conf.d", "20-bad.conf"),
			Contents: ciphertext,
		}},
	}); err == nil {
		t.Fatal("unbalanced quoting reached a configuration file")
	}

	if _, err := service.manager.Commit(storage.Request{
		Operation: "config.file_raw",
		Changes: []storage.Change{{
			Path:     filepath.Join(workspace.Root(), "sshc-notes.conf"),
			Contents: ciphertext,
		}},
	}); err == nil {
		t.Fatal("a sibling of sshc/ escaped validation")
	}
}

func TestStateOnlyWritesDoNotNeedAResolvableGraph(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	manager := storage.NewManager(workspace, time.Now, rand.Reader)
	_ = NewService(workspace, manager)

	if err := workspace.EnsureDirectory(workspace.StateDir()); err != nil {
		t.Fatal(err)
	}
	_, err = manager.Commit(storage.Request{
		Operation: "secret.vault",
		Changes: []storage.Change{{
			Path:     filepath.Join(workspace.StateDir(), "secrets"),
			Contents: []byte("sealed bytes that are not configuration"),
		}},
	})
	if err != nil {
		t.Fatalf("writing application state with no config present = %v", err)
	}
}

// known_hosts は設定のグラフに入らない。同じ storage.Manager を通る書き込みを、
// 設定にすでにある誤りを理由に断らない。断ると、~/.ssh/config の無いマシンで
// 初めて繋いだホストの鍵を保存できず、接続そのものが失敗する。
func TestKnownHostsWritesIgnoreErrorsAlreadyInTheConfiguration(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		config string
	}{
		{name: "no config"},
		{name: "include cycle", config: "Include config\n"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			workspace := newTestWorkspace(t)
			if testCase.config != "" {
				if err := os.WriteFile(filepath.Join(workspace.Root(), "config"), []byte(testCase.config), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			manager := storage.NewManager(workspace, time.Now, rand.Reader)
			_ = NewService(workspace, manager)
			hosts := knownhosts.NewService(workspace, manager, knownhosts.Scanner{})

			candidate := knownhosts.Candidate{
				Host: "bastion.example.com", Port: 22, KeyType: "ssh-ed25519",
				Key: "AAAAC3NzaC1lZDI1NTE5AAAAIPr0nHGmQb99GXmUofxJM4BXGwGzO0jGsQFBspODbkvS",
			}
			if _, err := hosts.Add(candidate, "", true); err != nil {
				t.Fatalf("adding a host key = %v", err)
			}
		})
	}
}

// 計画を通らない書き込みも、ディスクにすでにある誤りだけを見逃す。新しく
// 持ち込んだ誤りは断る。
func TestUnplannedWritesStillRefuseErrorsTheyIntroduce(t *testing.T) {
	workspace := newTestWorkspace(t)
	entry := filepath.Join(workspace.Root(), "config")
	if err := os.WriteFile(entry, []byte("Host nas\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := storage.NewManager(workspace, time.Now, rand.Reader)
	_ = NewService(workspace, manager)

	_, err := manager.Commit(storage.Request{
		Operation: "config.file_raw",
		Changes: []storage.Change{{
			Path: entry, Contents: []byte("Include config\nHost nas\n"),
			Precondition: storage.Precondition{Exists: true, Digest: storage.Digest([]byte("Host nas\n"))},
		}},
	})
	var graphError *GraphError
	if !errors.As(err, &graphError) {
		t.Fatalf("writing an include cycle = %v, want a GraphError", err)
	}
}

func TestKnownHostsWhoseCommentsHoldQuotesStillTakeAddedAndRememberedKeys(t *testing.T) {
	service, workspace := newTestService(t)
	hosts := knownhosts.NewService(workspace, service.manager, knownhosts.Scanner{})
	const existingKey = "AAAAC3NzaC1lZDI1NTE5AAAAIGZpeHR1cmVrZXlmaXh0dXJla2V5Zml4dHVyZWtl"
	// ssh_config の字句解析で読むと、どちらの行も閉じない引用になる。
	commented := "old.example ssh-ed25519 " + existingKey + " aida's laptop\n" +
		"legacy.example ssh-ed25519 " + existingKey + " the \"work\n"
	for _, name := range []string{"known_hosts", "known_hosts2"} {
		if err := os.WriteFile(filepath.Join(workspace.Root(), name), []byte(commented), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	candidate := knownhosts.Candidate{
		Host: "new.example", Port: 22, KeyType: "ssh-ed25519",
		Key: "AAAAC3NzaC1lZDI1NTE5AAAAIPr0nHGmQb99GXmUofxJM4BXGwGzO0jGsQFBspODbkvS",
	}

	if _, err := hosts.Add(candidate, "", true); err != nil {
		t.Fatalf("Add = %v", err)
	}
	if err := hosts.Remember(filepath.Join(workspace.Root(), "known_hosts2"), candidate); err != nil {
		t.Fatalf("Remember = %v", err)
	}

	const added = "new.example ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIPr0nHGmQb99GXmUofxJM4BXGwGzO0jGsQFBspODbkvS\n"
	for _, name := range []string{"known_hosts", "known_hosts2"} {
		if got := readFile(t, workspace, name); got != commented+added {
			t.Errorf("%s =\n%s\nwant\n%s", name, got, commented+added)
		}
	}
}

// known_hosts の操作は中身を ssh_config として検証しないので、UserKnownHostsFile が
// 誤って設定ファイルや metadata.json を指していても、鍵の行を足す先だけは断る。
func TestAKnownHostsOperationDoesNotWriteAConfigurationFileOrMetadata(t *testing.T) {
	service, workspace := newTestService(t)
	hosts := knownhosts.NewService(workspace, service.manager, knownhosts.Scanner{})
	metadata := writeServiceMetadata(t, service, workspace, NewMetadata())
	candidate := knownhosts.Candidate{
		Host: "new.example", Port: 22, KeyType: "ssh-ed25519",
		Key: "AAAAC3NzaC1lZDI1NTE5AAAAIPr0nHGmQb99GXmUofxJM4BXGwGzO0jGsQFBspODbkvS",
	}

	for _, relative := range []string{"config", "conf.d/10-home.conf"} {
		before := readFile(t, workspace, relative)
		if err := hosts.Remember(filepath.Join(workspace.Root(), filepath.FromSlash(relative)), candidate); !errors.Is(err, ErrNotEditable) {
			t.Errorf("Remember into %s = %v, want ErrNotEditable", relative, err)
		}
		if after := readFile(t, workspace, relative); after != before {
			t.Errorf("%s was changed to\n%s", relative, after)
		}
	}
	if err := hosts.Remember(service.metadata.Path(), candidate); !errors.Is(err, ErrNotEditable) {
		t.Errorf("Remember into metadata.json = %v, want ErrNotEditable", err)
	}
	if after, err := os.ReadFile(service.metadata.Path()); err != nil || string(after) != string(metadata) {
		t.Errorf("metadata.json = %q, %v; want it unchanged", after, err)
	}
}
