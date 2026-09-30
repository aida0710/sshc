package vpn

import (
	"regexp"
	"strings"
	"testing"
)

// イメージの apt は、sources に並べた suite ごとに、InRelease の SHA-256 を照合してから
// パッケージを入れる。suite を足して固定を忘れると、その suite の索引は署名だけで
// 受け入れられ、別の時刻の索引に差し替えられても気づけない。
func TestEverySnapshotSuiteHasAPinnedInRelease(t *testing.T) {
	contents, err := container.ReadFile("container/Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	dockerfile := string(contents)
	suitesLine := regexp.MustCompile(`'Suites: ([^']+)'`).FindStringSubmatch(dockerfile)
	if suitesLine == nil {
		t.Fatal("Dockerfile に Suites の行が無い")
	}
	suites := strings.Fields(suitesLine[1])
	if len(suites) == 0 {
		t.Fatal("Suites が空である")
	}
	for _, suite := range suites {
		pinned := regexp.MustCompile(`"[0-9a-f]{64}  \$\{lists\}_` + regexp.QuoteMeta(suite) + `_InRelease"`)
		if !pinned.MatchString(dockerfile) {
			t.Errorf("suite %s の InRelease の SHA-256 が固定されていない", suite)
		}
	}
	if !strings.Contains(dockerfile, "sha256sum --check --strict -") {
		t.Error("固定した SHA-256 を照合していない")
	}
}
