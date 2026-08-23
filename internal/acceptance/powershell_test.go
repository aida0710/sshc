package acceptance_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// **Windows PowerShell 5.1 は、BOM の無い .ps1 を ANSI として読む。**
//
// このリポジトリの注釈は日本語なので、BOM が無いと 5.1 の構文解析器が
// 日本語のバイトを現地のコードページとして読み、引用符や改行を飲み込んで
// 構文を壊す。落ち方が「Try に Catch も Finally も無い」のような、直接の
// 原因を指さないものになるうえ、pwsh 7 で書いている側では再現しない。
//
// **これは実機が教えたことである。** ある .ps1 は pwsh 7 で解析が通っていたのに、
// Windows 11 の既定の PowerShell では一行も走らなかった。
func TestWindowsPowerShellScriptsCarryABOMWhenTheyNeedOne(t *testing.T) {
	directory := filepath.Join("..", "..", "scripts", "windows")
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Skip("no Windows script directory in this checkout")
	}

	checked := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".ps1") {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		checked++

		// ASCII だけなら、どのコードページで読まれても同じものになる。
		ascii := true
		for _, b := range contents {
			if b > 0x7f {
				ascii = false
				break
			}
		}
		// BOM はバイト列として書く。Go のソースに直接置くと、そのファイル自身が
		// BOM を持っているように見える。
		hasBOM := bytes.HasPrefix(contents, []byte{0xef, 0xbb, 0xbf})
		if ascii {
			continue
		}
		if !hasBOM {
			t.Errorf("%s has non-ASCII content and no UTF-8 BOM; Windows PowerShell 5.1 will misread it", entry.Name())
		}
	}
	if checked == 0 {
		t.Fatal("no PowerShell scripts were checked; this test is looking in the wrong place")
	}
}
