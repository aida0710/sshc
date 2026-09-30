package keys

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sshc/internal/configresolver"
	"sshc/internal/storage"
)

// replaceBeforeReadFileSystem は、path を読む readsBeforeReplace+1 回目の直前に
// replace を一度だけ走らせる。インベントリの走査と agent への登録のあいだに、
// ほかのプロセスがファイルを差し替えた状況を作る。
type replaceBeforeReadFileSystem struct {
	storage.FileSystem
	path               string
	readsBeforeReplace int
	replace            func()
	reads              int
}

func (f *replaceBeforeReadFileSystem) ReadFile(path string) ([]byte, error) {
	if path == f.path {
		if f.reads == f.readsBeforeReplace && f.replace != nil {
			f.replace()
			f.replace = nil
		}
		f.reads++
	}
	return f.FileSystem.ReadFile(path)
}

// newRegisterRaceService は、~/.ssh/id_work を持つサービスを返す。id_work は、
// 走査で一度読まれたあと、次に読まれる直前に replace で差し替わる。
func newRegisterRaceService(t *testing.T, agent *fakeAgent, replace func(keyPath string)) *Service {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(home, ".ssh", "id_work")
	fileSystem := &replaceBeforeReadFileSystem{FileSystem: storage.OSFileSystem{}, path: keyPath}
	workspace, err := storage.NewWorkspace(fileSystem, home)
	if err != nil {
		t.Fatal(err)
	}
	manager := storage.NewManager(workspace, steppingClock(time.Date(2026, 8, 5, 9, 0, 0, 0, time.UTC)), rand.Reader)
	manager.Seal = sealForTest
	manager.Unseal = unsealForTest
	service := NewService(ServiceOptions{
		Workspace: workspace, Transactions: manager, Resolver: configresolver.ForWorkspace(workspace),
		Catalogue: CatalogueReader{Toolchain: fakeToolchain{}}, Agent: agent,
		Now: time.Now, Random: rand.Reader,
	})
	if _, err := service.Generate(GenerateRequest{
		Algorithm: AlgorithmEd25519, FileName: "id_work", Unencrypted: true,
	}); err != nil {
		t.Fatal(err)
	}
	fileSystem.reads = 0
	fileSystem.readsBeforeReplace = 1
	fileSystem.replace = func() { replace(keyPath) }
	return service
}

// writeOtherKey は、root の下に別の鍵を書いてそのパスを返す。
func writeOtherKey(t *testing.T, root string) string {
	t.Helper()
	private, err := GeneratePrivateKey(AlgorithmEd25519, 0, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := EncodePrivateKey(private, "other", nil)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "id_other")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// 走査のあとで鍵がワークスペースの外の鍵へのシンボリックリンクに差し替わっても、
// 登録はリンクをたどらず、agent へは何も渡さない。
func TestRegisterDoesNotFollowAKeyReplacedByASymlinkAfterTheScan(t *testing.T) {
	agent := &fakeAgent{available: true}
	outside := writeOtherKey(t, t.TempDir())
	service := newRegisterRaceService(t, agent, func(keyPath string) {
		if err := os.Remove(keyPath); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, keyPath); err != nil {
			t.Skipf("symlinks are unavailable here: %v", err)
		}
	})

	_, err := service.Register(context.Background(), RegisterRequest{KeyID: ItemID("id_work")})
	if !errors.Is(err, storage.ErrSymlinkPath) {
		t.Fatalf("Register = %v, want the symlink refused", err)
	}
	if len(agent.requests) != 0 {
		t.Fatalf("the agent received %d key(s) through a symlink", len(agent.requests))
	}
}

// 走査のあとで鍵が別の鍵に書き換わったら、走査で確かめたものと違う鍵を登録しない。
func TestRegisterRefusesAKeyThatChangedAfterTheScan(t *testing.T) {
	agent := &fakeAgent{available: true}
	service := newRegisterRaceService(t, agent, func(keyPath string) {
		other, err := os.ReadFile(writeOtherKey(t, t.TempDir()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(keyPath, other, 0o600); err != nil {
			t.Fatal(err)
		}
	})

	_, err := service.Register(context.Background(), RegisterRequest{KeyID: ItemID("id_work")})
	if !errors.Is(err, ErrKeyChanged) {
		t.Fatalf("Register = %v, want ErrKeyChanged", err)
	}
	if len(agent.requests) != 0 {
		t.Fatalf("the agent received %d key(s) that the scan never saw", len(agent.requests))
	}
}
