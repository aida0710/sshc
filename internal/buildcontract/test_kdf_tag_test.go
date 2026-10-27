package buildcontract

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// testKDFTag は、テストで Argon2id のコストを下げるビルドタグである
// （internal/envelope/derivation_cost_sshc_test_kdf.go）。
const testKDFTag = "sshc_test_kdf"

// testKDFTagFiles は、testKDFTag を書いてよいファイルである。テストを流す命令（CI の
// go job と make test）と、タグの実装・説明・この検査に限る。
var testKDFTagFiles = []string{
	".github/workflows/ci.yml",
	"Makefile",
	"internal/buildcontract/test_kdf_tag_test.go",
	"internal/buildcontract/workflow_test.go",
	"internal/envelope/derivation_cost_sshc_test_kdf.go",
	"internal/envelope/envelope.go",
	"internal/envelope/main_test.go",
}

// 製品のバイナリをタグ sshc_test_kdf でビルドすると、Vault と同期の鍵がほぼ無料で
// 総当たりできるものになる。リリースの workflow、nativebuild、Homebrew の formula、
// インストーラーなどがこのタグを使わないよう、タグを書いてよいファイルを限る。
func TestTheTestKDFTagStaysOutOfEveryReleaseBuild(t *testing.T) {
	root := filepath.Join("..", "..")
	var found []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "dist", "artifacts":
				return filepath.SkipDir
			}
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(contents), testKDFTag) {
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			found = append(found, filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range found {
		if !slices.Contains(testKDFTagFiles, path) {
			t.Errorf("%s mentions the build tag %s, which lowers the key derivation cost; only the test commands may use it", path, testKDFTag)
		}
	}

	// Makefile では、test の target の go test だけが使う。
	for line := range strings.Lines(readContractFile(t, "Makefile")) {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, testKDFTag) && !strings.HasPrefix(trimmed, "@#") &&
			trimmed != "go test -count=1 -race -tags "+testKDFTag+" ./..." {
			t.Errorf("Makefile uses %s outside the go test of the test target: %q", testKDFTag, trimmed)
		}
	}
}
