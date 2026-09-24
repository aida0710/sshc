package buildcontract

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// markdownLinkTarget は、Markdown のリンクと画像の飛び先を取り出す。
var markdownLinkTarget = regexp.MustCompile(`\]\(([^)\s]+)`)

// リリースノートは GitHub Release の本文として公開される。
//
// 本文の相対リンクは、Release のページではリポジトリの直下からの位置として
// 読まれる。docs/releases/ の中では正しいリンクが、公開したページでは 404 になる。
func TestReleaseNotesLinkOnlyToAbsoluteURLs(t *testing.T) {
	notes, err := filepath.Glob(filepath.Join("..", "..", "docs", "releases", "v*.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) == 0 {
		t.Fatal("no release notes found under docs/releases")
	}
	for _, path := range notes {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range markdownLinkTarget.FindAllStringSubmatch(string(contents), -1) {
			target := match[1]
			if strings.HasPrefix(target, "https://") || strings.HasPrefix(target, "#") {
				continue
			}
			t.Errorf("%s links to %q; a relative link breaks on the GitHub Release page", filepath.Base(path), target)
		}
	}
}
