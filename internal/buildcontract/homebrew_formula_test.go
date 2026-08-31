package buildcontract

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// formula が建てようとするものが、実際に建つことを確かめる。
//
// v0.3.1 の `brew install` は "no Go files in ..." で落ちた。formula は
// `go build` に package を渡しておらず、tarball の root を建てようとしていた
// main は ./cmd/sshc に居る。リリースは緑のまま出ており、気付いたのは
// 利用者が打ったときだった。
//
// ここは brew を持たない機械でも走る。読むのは formula に書いてある文字列で
// あり、それを同じ引数で実行してみるだけである。
func TestTheFormulaBuildsSomethingThatExists(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "packaging", "homebrew", "sshc.rb"))
	if err != nil {
		t.Fatal(err)
	}
	formula := string(body)

	// `system "go", "build", *std_go_args(...), "./cmd/sshc"` の最後の引数。
	build := regexp.MustCompile(`system "go", "build",[^\n]*`).FindString(formula)
	if build == "" {
		t.Fatal("formula に go build の行が無い")
	}
	packages := regexp.MustCompile(`"(\./[^"]+)"`).FindStringSubmatch(build)
	if packages == nil {
		t.Fatalf("formula は go build に package を渡していない: %s\n"+
			"渡さないと tarball の root を建てようとして \"no Go files\" で止まる", build)
	}

	// その package が本当に main であることまで見る。表記だけ合っていて
	// 中身が library なら、brew は同じ形で落ちる。
	// リポジトリの根で訊く。formula の表記はあそこを基点にしている。
	list := exec.Command("go", "list", "-f", "{{.Name}}", packages[1])
	list.Dir = filepath.Join("..", "..")
	output, err := list.CombinedOutput()
	if err != nil {
		t.Fatalf("formula が名指す %s を go list が読めない: %v\n%s", packages[1], err, output)
	}
	if name := strings.TrimSpace(string(output)); name != "main" {
		t.Errorf("formula は %s を建てようとしているが、あれは package %s である", packages[1], name)
	}

	// std_go_args は -s -w を自分で足す。重ねて書くと二重になる。
	if strings.Contains(build, `-s -w`) {
		t.Error("formula の ldflags が -s -w を重ねている: std_go_args が既に足す")
	}
}

func TestTheFormulaInstallsShellCompletions(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "packaging", "homebrew", "sshc.rb"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `generate_completions_from_executable(bin/"sshc", "completion")`) {
		t.Error("formula が sshc completion から bash/zsh/fish の補完を生成していない")
	}
}

// 利用者に打たせる行は、打てば通る行でなければならない。
//
// docs は一度 `brew install --cask --no-quarantine ...` を案内していた。
// あの旗は Homebrew 5.0 で非推奨、5.1 で削除されており、打ったユーザーが受け取るのは
// 使い方の全文と `Error: invalid option` である。確かめずに書いた。
//
// ここが数えるのは、案内の中の brew の行が、消えた旗を含まないことである。
func TestTheInstructionsUseNoRetiredBrewFlags(t *testing.T) {
	// 消えたもの。増えたらここへ足す。
	retired := map[string]string{
		"--no-quarantine": "Homebrew 5.1 で削除された（Gatekeeper の迂回を提供しない方針。代替なし）",
		"--quarantine":    "同上",
	}
	for _, name := range []string{
		filepath.Join("..", "..", "docs", "release-install.md"),
		filepath.Join("..", "..", "README.md"),
	} {
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for index, line := range strings.Split(string(body), "\n") {
			if !strings.Contains(line, "brew ") {
				continue
			}
			// 「もう無い」と説明している行は案内ではない。
			if strings.Contains(line, "削除") || strings.Contains(line, "非推奨") {
				continue
			}
			for flag, why := range retired {
				if strings.Contains(line, flag) {
					t.Errorf("%s:%d が %s を打たせている: %s\n  %s",
						filepath.Base(name), index+1, flag, why, strings.TrimSpace(line))
				}
			}
		}
	}
}
