package process_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"sshc/internal/platform"
	"sshc/internal/platform/process"
)

var _ platform.Toolchain = process.Toolchain{}

func TestToolchainResolvesEveryProgramThroughTheInjectedStat(t *testing.T) {
	// 探索の起点はこのファイルシステムの絶対パスでなければならない。Toolchain は
	// filepath.Join で候補を組み立てるので、Unix 綴りの `/sandbox` を渡すと
	// Windows では区切り文字が変わり、fstest 側の鍵と一致しなくなる。
	sandbox := filepath.Join(filepath.VolumeName(os.TempDir())+string(os.PathSeparator), "sandbox")
	installed := fstest.MapFS{"sandbox/ssh-keygen": &fstest.MapFile{Mode: 0o755}}
	var asked []string
	toolchain := process.Toolchain{
		Directories: []string{sandbox},
		Stat: func(name string) (fs.FileInfo, error) {
			asked = append(asked, name)
			return installed.Stat(filepath.ToSlash(strings.TrimPrefix(name, filepath.VolumeName(name)+string(os.PathSeparator))))
		},
	}

	resolvers := map[string]func() (string, error){"ssh-keygen": toolchain.KeyGen}
	for program, resolve := range resolvers {
		path, err := resolve()
		if err != nil {
			t.Fatalf("resolving %s = %v", program, err)
		}
		if want := filepath.Join(sandbox, program); path != want {
			t.Errorf("resolving %s = %q, want %q", program, path, want)
		}
	}
	if len(asked) != len(resolvers) {
		t.Errorf("injected Stat saw %#v, want one lookup per program", asked)
	}
}
