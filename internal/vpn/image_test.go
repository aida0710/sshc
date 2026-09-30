package vpn

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dockerTestImageName は、VPN の結合テストが作って使うイメージの名前である。イメージを
// 作ると同じ名前のほかのタグを消すので、インストールした sshc のイメージ
// （defaultImageName）と名前を分ける。
const dockerTestImageName = "sshc-vpn-test"

// newDockerTestManager は、結合テストの Manager を作る。結合テストは Linux だけで
// 走るが、名前を分けたことはどの OS のテストでも確かめるので、ここに置く。
func newDockerTestManager(t *testing.T) *Manager {
	t.Helper()
	manager := New(t.TempDir(), os.Getuid(), nil)
	manager.imageName = dockerTestImageName
	return manager
}

// イメージを作ったあとに消すのは、この Manager の名前のイメージだけである。結合テストの
// 名前で作ったときに、インストールした sshc の sshc-vpn のイメージへ触れない。
func TestRemovingOtherImagesTouchesOnlyImagesOfTheSameName(t *testing.T) {
	calls := filepath.Join(t.TempDir(), "calls")
	docker := fakeDocker(t, `echo "$*" >>`+calls+`
if [ "$1 $2 $3" = "image ls sshc-vpn-test" ]; then
	printf '%s\n' sshc-vpn-test:current sshc-vpn-test:stale
fi
`)
	manager := &Manager{docker: docker, imageName: dockerTestImageName}

	manager.removeOtherImages(context.Background(), "sshc-vpn-test:current")

	contents, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimSpace(string(contents)), "\n")
	want := []string{"image ls sshc-vpn-test --format {{.Repository}}:{{.Tag}}", "image rm sshc-vpn-test:stale"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("docker に渡した引数 = %q, want %q", got, want)
	}
}

// 結合テストの Manager は、インストールした sshc と違う名前のイメージを使う。
func TestTheDockerTestsUseTheirOwnImageName(t *testing.T) {
	if manager := newDockerTestManager(t); manager.imageName == defaultImageName {
		t.Fatalf("結合テストのイメージの名前が %q で、インストールした sshc と同じ", manager.imageName)
	}
	if manager := New(t.TempDir(), 1000, nil); manager.imageName != defaultImageName {
		t.Fatalf("New のイメージの名前 = %q, want %q", manager.imageName, defaultImageName)
	}
}
