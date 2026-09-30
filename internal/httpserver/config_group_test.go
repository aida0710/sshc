package httpserver

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"golang.org/x/crypto/ssh"

	"sshc/internal/application"
	"sshc/internal/configresolver"
	"sshc/internal/keys"
	"sshc/internal/storage"
)

// newGroupHarness は、グループ操作が鍵の一覧を読めるように Keys を渡した config の経路を組む。
// session と CSRF の検査は TestConfigEndpointsRequireASessionAndCSRF が受け持つので、ここでは省く。
func newGroupHarness(t *testing.T, groups ...string) *testHarness {
	t.Helper()
	harness := newConfigHarness(t)
	keyService := keys.NewService(keys.ServiceOptions{
		Workspace:    harness.workspace,
		Transactions: storage.NewManager(harness.workspace, nil, rand.Reader),
		Resolver:     configresolver.ForWorkspace(harness.workspace),
	})
	engine := echo.New()
	registerConfigRoutes(engine, ConfigHandlers{Service: harness.service, Keys: keyService})
	harness.echo = engine

	metadata := application.NewMetadata()
	for _, name := range groups {
		metadata.Groups = append(metadata.Groups, application.GroupMetadata{Name: name})
	}
	if _, err := harness.service.Save(application.EditRequest{Kind: application.EditGroups, Metadata: &metadata}); err != nil {
		t.Fatalf("declare %v: %v", groups, err)
	}
	return harness
}

// writeWorkspaceFile は、~/.ssh の下へ利用者だけが読めるファイルを置く。
func writeWorkspaceFile(t *testing.T, workspace *storage.Workspace, relative string, contents []byte) {
	t.Helper()
	absolute := filepath.Join(workspace.Root(), filepath.FromSlash(relative))
	if err := workspace.EnsureDirectory(filepath.Dir(absolute)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absolute, contents, 0o600); err != nil {
		t.Fatal(err)
	}
}

// writeGroupKeyPair は、relative に秘密鍵を、その横に誰でも読める公開鍵（.pub）を置く。
func writeGroupKeyPair(t *testing.T, workspace *storage.Workspace, relative string) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(private, "")
	if err != nil {
		t.Fatal(err)
	}
	sshPublic, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	writeWorkspaceFile(t, workspace, relative, pem.EncodeToMemory(block))
	publicPath := filepath.Join(workspace.Root(), filepath.FromSlash(relative+".pub"))
	if err := os.WriteFile(publicPath, ssh.MarshalAuthorizedKey(sshPublic), 0o644); err != nil {
		t.Fatal(err)
	}
}

// groupKeyInsideGroupHarness は、グループ work の接続 web-1 が、同じグループの鍵
// keys/work/id_work を指している配置を作る。移動先のグループ archive も宣言する。
func groupKeyInsideGroupHarness(t *testing.T) *testHarness {
	t.Helper()
	harness := newGroupHarness(t, "work", "archive")
	writeWorkspaceFile(t, harness.workspace, "connections/work/web.conf",
		[]byte("Host web-1\n\tHostName 203.0.113.10\n\tIdentityFile ~/.ssh/keys/work/id_work\n"))
	writeGroupKeyPair(t, harness.workspace, "keys/work/id_work")
	return harness
}

// assertMovedConnectionNamesKey は、グループと一緒に移った web-1 の設定が、移った先の
// グループの鍵を指していることを確かめる。
func assertMovedConnectionNamesKey(t *testing.T, harness *testHarness, group string) {
	t.Helper()
	moved, err := os.ReadFile(filepath.Join(harness.root, "connections", group, "web.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "IdentityFile ~/.ssh/keys/" + group + "/id_work\n"; !strings.Contains(string(moved), want) {
		t.Errorf("the moved connection still names the old key path: %q", moved)
	}
}

func TestGroupRenameSucceedsWhenAConnectionInTheGroupNamesTheGroupsKey(t *testing.T) {
	harness := groupKeyInsideGroupHarness(t)

	response := harness.call(t, http.MethodPost, "/api/v1/config/groups/rename",
		map[string]string{"from": "work", "to": "client-a"}, true, true)
	if response.Code != http.StatusOK {
		t.Fatalf("rename status = %d, body = %s", response.Code, response.Body.String())
	}
	assertMovedConnectionNamesKey(t, harness, "client-a")
}

func TestGroupDeleteSucceedsWhenAConnectionInTheGroupNamesTheGroupsKey(t *testing.T) {
	harness := groupKeyInsideGroupHarness(t)

	response := harness.call(t, http.MethodPost, "/api/v1/config/groups/delete",
		map[string]string{"name": "work", "destination": "archive"}, true, true)
	if response.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", response.Code, response.Body.String())
	}
	assertMovedConnectionNamesKey(t, harness, "archive")
}
