package vpn

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// dockerContainerName は、docker がコンテナ名として受け付ける形である。
var dockerContainerName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]+$`)

// 空白や日本語を含む名前でも、コンテナ名と経路の置き場所は docker とファイルシステムが
// そのまま受け付ける字だけになる。
func TestANameWithSpacesAndJapaneseMakesUsableContainerAndDirectoryNames(t *testing.T) {
	manager := New(t.TempDir(), 1000, nil)
	name := "研究室 VPN（本郷）"

	if container := manager.containerName(name); !dockerContainerName.MatchString(container) {
		t.Errorf("コンテナ名 %q を docker が受け付けない", container)
	}
	directory := filepath.Base(manager.routeDirectory(name))
	if !regexp.MustCompile(`^[0-9a-f]+$`).MatchString(directory) {
		t.Errorf("経路の置き場所 %q に名前の字が入った", directory)
	}
}

// 大文字と小文字だけが違う名前の経路は、大文字と小文字を区別しないファイルシステム
// （macOS と Windows の既定）でも別の置き場所になる。
func TestNamesDifferingOnlyInCaseGetSeparateRouteDirectories(t *testing.T) {
	manager := New(t.TempDir(), 1000, nil)

	upper, lower := manager.routeDirectory("Lab"), manager.routeDirectory("lab")

	if strings.EqualFold(upper, lower) {
		t.Fatalf("Lab と lab の置き場所が重なる: %q, %q", upper, lower)
	}
}

// コンテナがどのプロファイルのものかは、名前そのものを持つ札で確かめる。空白を含む
// 名前も、区切りと取り違えない。
func TestAContainerIsMatchedToItsProfileByItsLabel(t *testing.T) {
	manager := New(t.TempDir(), 1000, nil)
	labels := `{"io.sshc.vpn.owner":"1000","io.sshc.vpn.profile":"研究室 VPN","io.sshc.vpn.workspace":"` +
		manager.workspace + `"}`
	manager.docker = fakeDocker(t, "echo '"+labels+"'\n")
	container := manager.containerName("研究室 VPN")

	ours, err := manager.requireOurContainer(context.Background(), container, "研究室 VPN")
	if err != nil || !ours {
		t.Fatalf("requireOurContainer = %v, %v", ours, err)
	}
	if _, err := manager.requireOurContainer(context.Background(), container, "研究室"); !errors.Is(err, ErrContainerForeign) {
		t.Fatalf("別のプロファイルの札のコンテナ = %v, want ErrContainerForeign", err)
	}
}

// 前のバージョンの名前の付け方（sshc-vpn-<プロファイル名>-…）で残ったコンテナも、起動時の
// 回収で止める。回収はコンテナ名ではなく、利用者と workspace の札で探す。
func TestOrphansAreFoundByLabelWhateverTheirContainerName(t *testing.T) {
	calls := filepath.Join(t.TempDir(), "calls")
	manager := New(t.TempDir(), 1000, nil)
	manager.docker = fakeDocker(t, `echo "$@" >> '`+calls+`'
if [ "$1" = ps ]; then echo sshc-vpn-lab-1000-`+manager.workspace+`; fi
exit 0
`)
	manager.found = true

	if err := manager.DiscardOrphans(context.Background()); err != nil {
		t.Fatalf("DiscardOrphans = %v", err)
	}

	contents, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	if !strings.Contains(text, "label="+ownerLabel+"=1000") || !strings.Contains(text, "label="+workspaceLabel+"="+manager.workspace) ||
		strings.Contains(text, "name=") {
		t.Errorf("回収のための docker ps が札だけで探していない:\n%s", text)
	}
	stopped := regexp.MustCompile(`(?m)^stop .*sshc-vpn-lab-1000-` + manager.workspace + `$`)
	if !stopped.MatchString(text) {
		t.Errorf("前の名前の付け方のコンテナを止めなかった:\n%s", text)
	}
}
