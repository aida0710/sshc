package buildcontract

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// formula が建てようとするものが、実際に建つことを確かめる。
//
// **v0.3.1 の `brew install` は "no Go files in ..." で落ちた。** formula は
// `go build` に package を渡しておらず、tarball の root を建てようとしていた
// ——main は ./cmd/sshc に居る。リリースは緑のまま出ており、**気付いたのは
// 利用者が打ったときだった。**
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

	// **その package が本当に main であることまで見る。** 綴りだけ合っていて
	// 中身が library なら、brew は同じ形で落ちる。
	// **リポジトリの根で訊く。** formula の綴りはあそこを基点にしている。
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

// cask が指す dmg の名前は、リリースが実際に作る名前でなければならない。
//
// **formula が建てられないまま出ていったのと同じ穴である。** cask の側は
// 建てるものが無いぶん確かめる先が少なく、名前がずれれば 404 になる——
// リリースは緑で終わり、気付くのは利用者が打ったときになる。
func TestTheCaskNamesTheDiskImageTheReleaseBuilds(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "packaging", "homebrew", "sshc-cask.rb"))
	if err != nil {
		t.Fatal(err)
	}
	cask := string(body)

	// electron-builder の artifactName がその名前を決めている。
	manifest, err := os.ReadFile(filepath.Join("..", "..", "desktop", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var desktop struct {
		ProductName string `json:"productName"`
		Build       struct {
			Mac struct {
				ArtifactName string `json:"artifactName"`
			} `json:"mac"`
		} `json:"build"`
	}
	if err := json.Unmarshal(manifest, &desktop); err != nil {
		t.Fatal(err)
	}

	// ${productName}-${version}-${os}-${arch}.${ext} を cask の綴りへ写す。
	expected := desktop.Build.Mac.ArtifactName
	expected = strings.ReplaceAll(expected, "${productName}", desktop.ProductName)
	expected = strings.ReplaceAll(expected, "${version}", "#{version}")
	expected = strings.ReplaceAll(expected, "${os}", "mac")
	expected = strings.ReplaceAll(expected, "${arch}", "#{arch}")
	expected = strings.ReplaceAll(expected, "${ext}", "dmg")
	if !strings.Contains(cask, expected) {
		t.Errorf("cask が指す名前と、リリースが作る名前が違う\n  リリース: %s\n  cask   : %s",
			expected, regexp.MustCompile(`url "[^"]+"`).FindString(cask))
	}

	// **入れるのは .app ひとつである。** productName が変われば束の中の名前も変わる。
	if !strings.Contains(cask, `app "`+desktop.ProductName+`.app"`) {
		t.Errorf("cask が入れようとしている .app の名前が productName（%s）と違う", desktop.ProductName)
	}

	// **CLI は formula の担当である。** cask が同じ名前を PATH へ張ると、
	// brew が同じ綴りを二度管理することになる。
	if strings.Contains(cask, "binary ") {
		t.Error("cask が binary を張っている: 端末側の入口は formula が持つ")
	}
}
