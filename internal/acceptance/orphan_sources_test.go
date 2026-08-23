package acceptance_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// **Go でない原文が、連れの Go を失ったまま残ることがある。**
//
// これは実際に起きた。生体認証を畳んだとき `biometric_darwin.go` と `.h` は消せた
// のに `.m` が残り、**Linux では一度も見えなかった** ——`//go:build darwin` が
// パッケージごと除外するからである。macOS の CI が初めてこう言った:
//
//	Objective-C source files not allowed when not using cgo or SWIG
//
// 手元の OS では通り、別の OS でだけ落ちる種類の残骸なので、**ファイルの並びから
// 見つけるしかない。** cgo の原文は、それを `import "C"` する Go と対でしか
// 意味を持たない——連れが居ないなら、それは誰も読まないバイト列である。
func TestNoCompiledSourceOutlivesItsGoCompanion(t *testing.T) {
	root := filepath.Join("..", "..")
	// **並べるのは cgo が拾う拡張子である。** 拾わないものは、置いてあっても
	// ビルドに入らないので、ここが見張る対象ではない。
	compiled := map[string]bool{".m": true, ".mm": true, ".c": true, ".cc": true, ".cpp": true, ".h": true, ".hpp": true, ".s": true}

	orphans := []string{}
	checked := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "dist", "build", "android":
				return filepath.SkipDir
			}
			return nil
		}
		if !compiled[strings.ToLower(filepath.Ext(entry.Name()))] {
			return nil
		}
		checked++
		// 同じディレクトリに `import "C"` する Go が一つでもあれば、連れは居る。
		siblings, err := os.ReadDir(filepath.Dir(path))
		if err != nil {
			return err
		}
		for _, sibling := range siblings {
			if !strings.HasSuffix(sibling.Name(), ".go") {
				continue
			}
			body, err := os.ReadFile(filepath.Join(filepath.Dir(path), sibling.Name()))
			if err != nil {
				continue
			}
			if strings.Contains(string(body), `import "C"`) {
				return nil
			}
		}
		relative, _ := filepath.Rel(root, path)
		orphans = append(orphans, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(orphans) != 0 {
		t.Errorf("these are compiled by cgo but no Go file here imports \"C\":\n  %s",
			strings.Join(orphans, "\n  "))
	}
	// **0 件で緑になるのは、歩き方を間違えたときも同じである。**
	if checked == 0 {
		t.Log("no cgo sources in this checkout; the walk found nothing to judge")
	}
}
