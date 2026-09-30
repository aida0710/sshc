package application

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"sshc/internal/storage"
)

func writePrivateKey(t *testing.T, absolute string) string {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(private, "")
	if err != nil {
		t.Fatal(err)
	}
	contents := string(pem.EncodeToMemory(block))
	if err := os.WriteFile(absolute, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return contents
}

// Config Explorerの新規ファイル欄や改名欄に鍵やsshcの状態ファイルの名前を打ち込んでも、
// 設定ファイルとして読み書きしない。書き換えれば鍵やVaultが壊れ、衝突の差分には
// 秘密鍵の本文がそのまま載る。
func TestConfigExplorerRefusesFilesThatAreNotConfiguration(t *testing.T) {
	service, root := newFileOpsService(t)
	for _, directory := range []string{"sshc", filepath.Join("keys", "work")} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "sshc", "local-vault-key"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "known_hosts"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	key := writePrivateKey(t, filepath.Join(root, "conf.d", "deploy.conf"))
	created := "# created by sshc\n"

	tests := []struct {
		name    string
		request EditRequest
	}{
		{"new file over the empty vault key", EditRequest{Kind: EditFileRaw, Path: "sshc/local-vault-key", Raw: created}},
		{"new file over the empty known_hosts", EditRequest{Kind: EditFileRaw, Path: "known_hosts", Raw: created}},
		{"new file in the group key directory", EditRequest{Kind: EditFileRaw, Path: "keys/work/new", Raw: created}},
		{"stale edit of a private key", EditRequest{Kind: EditFileRaw, Path: "conf.d/deploy.conf", Raw: created}},
		{"rename of a private key", EditRequest{Kind: EditFileRename, Path: "conf.d/deploy.conf", DestinationPath: "conf.d/moved.conf"}},
		{"delete of a private key", EditRequest{Kind: EditFileDelete, Path: "conf.d/deploy.conf"}},
		{"rename of a configuration file into the state directory", EditRequest{
			Kind: EditFileRename, Path: "work/lon.conf", Base: readWorkspace(t, root, "work/lon.conf"),
			DestinationPath: "sshc/lon.conf",
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := service.Preview(test.request); !errors.Is(err, ErrNotEditable) {
				t.Fatalf("Preview = %v, want ErrNotEditable", err)
			}
			if _, err := service.Save(test.request); !errors.Is(err, ErrNotEditable) {
				t.Fatalf("Save = %v, want ErrNotEditable", err)
			}
		})
	}

	for _, relative := range []string{"sshc/local-vault-key", "known_hosts", "conf.d/deploy.conf"} {
		if _, err := service.FileContents(relative); !errors.Is(err, ErrNotEditable) {
			t.Errorf("FileContents(%s) = %v, want ErrNotEditable", relative, err)
		}
	}
	if got := readWorkspace(t, root, "sshc/local-vault-key"); got != "" {
		t.Errorf("the vault key was written: %q", got)
	}
	if got := readWorkspace(t, root, "conf.d/deploy.conf"); got != key {
		t.Error("the private key was changed")
	}
	if _, err := os.Stat(filepath.Join(root, "keys", "work", "new")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a file was created in the key directory: %v", err)
	}
}

// Includeから届かない場所にも、新しい設定ファイルは作れて、そのまま開いて直せる。
// Config Explorerは作ったファイルや、改名でIncludeから外れたファイルをその場で開く。
func TestConfigExplorerStillEditsAConfigurationFileOutsideTheIncludeGraph(t *testing.T) {
	service, root := newFileOpsService(t)
	created := "# created by sshc\n"
	if _, err := service.Save(EditRequest{Kind: EditFileRaw, Path: "drafts/new.conf", Raw: created}); err == nil {
		t.Fatal("Save into a missing directory succeeded")
	}
	if _, err := service.Save(EditRequest{Kind: EditFileRaw, Path: "draft.conf", Raw: created}); err != nil {
		t.Fatalf("Save(new file) = %v", err)
	}
	contents, err := service.FileContents("draft.conf")
	if err != nil {
		t.Fatalf("FileContents = %v", err)
	}
	if !contents.Editable || contents.Contents != created {
		t.Fatalf("FileContents = %+v, want the created file as editable", contents)
	}
	edited := created + "Host draft\n\tHostName 192.0.2.1\n"
	if _, err := service.Save(EditRequest{Kind: EditFileRaw, Path: "draft.conf", Base: created, Raw: edited}); err != nil {
		t.Fatalf("Save(edit) = %v", err)
	}
	if got := readWorkspace(t, root, "draft.conf"); !strings.Contains(got, "Host draft") {
		t.Fatalf("draft.conf = %q", got)
	}
}

// errPrivateStateUnreadable は、privateStateUnreadableFileSystem が状態ファイルの読み込みに返す。
var errPrivateStateUnreadable = errors.New("the test refuses to read private state")

// privateStateUnreadableFileSystem は、sshcの状態ディレクトリの読み込みをすべて失敗させる。
// WindowsのOSFileSystemは状態ファイルをACLを確かめながら読み、確認に落ちると読めない。
// FileSystemをinterfaceとして埋め込むので、OSFileSystemのReadPrivateFileは昇格しない。
type privateStateUnreadableFileSystem struct {
	storage.FileSystem
	privateReads []string
}

func (fileSystem *privateStateUnreadableFileSystem) ReadPrivateFile(path string) ([]byte, error) {
	fileSystem.privateReads = append(fileSystem.privateReads, path)
	return nil, errPrivateStateUnreadable
}

// Config Explorerは、sshcの状態ファイルを名前だけで断り、中身を読まない。読んでから
// 断ると、Windowsで状態ファイルのACLの確認に落ちたときに、path_not_editableではなく
// ACLのエラーが返る。断るだけの操作のためにVaultの鍵を読むことにもなる。
func TestConfigExplorerRefusesStateFilesWithoutReadingThem(t *testing.T) {
	fileSystem := &privateStateUnreadableFileSystem{FileSystem: storage.OSFileSystem{}}
	workspace, err := storage.NewWorkspace(fileSystem, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.EnsureDirectory(workspace.StateDir()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace.Root(), "config"), []byte("Host bastion\n\tHostName 203.0.113.10\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace.StateDir(), "local-vault-key"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader))

	requests := []EditRequest{
		{Kind: EditFileRaw, Path: "sshc/local-vault-key", Raw: "# created by sshc\n"},
		{Kind: EditFileRename, Path: "sshc/local-vault-key", DestinationPath: "vault.conf"},
		{Kind: EditFileDelete, Path: "sshc/local-vault-key"},
	}
	for _, request := range requests {
		if _, err := service.Preview(request); !errors.Is(err, ErrNotEditable) {
			t.Errorf("Preview(%s) = %v, want ErrNotEditable", request.Kind, err)
		}
	}
	if _, err := service.FileContents("sshc/local-vault-key"); !errors.Is(err, ErrNotEditable) {
		t.Errorf("FileContents = %v, want ErrNotEditable", err)
	}
	if len(fileSystem.privateReads) != 0 {
		t.Errorf("state files read = %v, want none", fileSystem.privateReads)
	}
}
