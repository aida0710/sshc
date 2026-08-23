package buildcontract

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// 書いてある手順は、打てなければならない。
//
// iOS をやめて `ios-bind` を消したとき、README と docs/design.md は
// `make ios-bind` を案内したまま残った。読んだユーザーが最初に打つのがそれである。
//
// この repo は同種のずれを軒並み検査で縛っている。生成物、API の契約、
// workflow の形、保管庫の寿命の表記。ここだけ穴だった。
//
// 記録は見ない。日付の付いた設計文書と superpowers の計画は、その日に
// 何を打ったかの記録であり、いま打てる必要はない。

// currentDocuments は、いま読まれて実行される文書である。
var currentDocuments = []string{
	"README.md",
	filepath.Join("docs", "design.md"),
	filepath.Join("docs", "release-install.md"),
	filepath.Join("docs", "headless-examples.md"),
	filepath.Join("docs", "manual-acceptance.md"),
	filepath.Join("docs", "manual-test-matrix.md"),
}

// makefileTargets は、Makefile が持っている的の集合を返す。
func makefileTargets(t *testing.T) map[string]bool {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	targets := map[string]bool{}
	for _, line := range strings.Split(string(body), "\n") {
		if match := regexp.MustCompile(`^([a-z][a-z0-9-]*):`).FindStringSubmatch(line); match != nil {
			targets[match[1]] = true
		}
	}
	if len(targets) == 0 {
		t.Fatal("Makefile から的をひとつも読めなかった")
	}
	return targets
}

func TestEveryDocumentedMakeTargetExists(t *testing.T) {
	targets := makefileTargets(t)
	// `make <的>` と書かれた形だけを見る。散文で的の名前に触れるのは構わない
	//「かつて ios-bind という的があった」は打てなくてよい。
	named := regexp.MustCompile(`make ([a-z][a-z0-9-]*)`)

	var missing []string
	for _, document := range currentDocuments {
		body, err := os.ReadFile(filepath.Join("..", "..", document))
		if err != nil {
			t.Fatalf("read %s: %v", document, err)
		}
		for _, match := range named.FindAllStringSubmatch(string(body), -1) {
			if !targets[match[1]] {
				missing = append(missing, document+" が `make "+match[1]+"` を案内しているが、Makefile に無い")
			}
		}
	}
	sort.Strings(missing)
	for _, problem := range missing {
		t.Error(problem)
	}
}
