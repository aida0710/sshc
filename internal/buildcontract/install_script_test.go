package buildcontract

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// install.sh は `curl | sh` で流し込まれる。**渡された側からは、それが何を
// するのか実行するまで見えない。** だからここで、見えないまま任せてよい性質だけを
// 持っていることを確かめる。
func readInstallScript(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// **script が知っている機械と、リリースが作る機械は同じでなければならない。**
//
// Makefile の RELEASE_TARGETS が正本である。増やしたのに script が知らなければ、
// その機械の利用者は「作られているのに落とせない」に当たる。減らしたのに script が
// 案内し続ければ、存在しない URL を叩いて 404 で終わる——どちらも、リリースの
// 側を直した人には見えない壊れ方である。
func TestTheInstallScriptKnowsEveryMachineTheReleaseBuilds(t *testing.T) {
	contract := readMakefileContract(t)
	targets := contract.variables["RELEASE_TARGETS"]
	if len(targets) == 0 {
		t.Fatal("RELEASE_TARGETS is missing from the Makefile")
	}
	script := readInstallScript(t)

	for _, target := range targets {
		platform, _, _ := strings.Cut(target, ":")
		goos, goarch, ok := strings.Cut(platform, "/")
		if !ok {
			t.Fatalf("RELEASE_TARGETS entry %q is not goos/goarch", target)
		}
		if goos == "windows" {
			// **Windows はこの script の担当ではない。** あちらはインストーラが
			// PATH まで通す。名指しでそう言っているかだけを見る。
			if !strings.Contains(script, "Windows has an installer") {
				t.Error("install.sh must send Windows users to the installer instead of failing silently")
			}
			continue
		}
		if !strings.Contains(script, fmt.Sprintf("goos=%s", goos)) {
			t.Errorf("install.sh does not map uname to %s, but the release builds it", goos)
		}
		if !strings.Contains(script, fmt.Sprintf("goarch=%s", goarch)) {
			t.Errorf("install.sh does not map uname to %s, but the release builds it", goarch)
		}
	}
}

// **確かめずに置かない。** 流し込む入れ方は途中ですり替えられても受け取った側に
// 見えないので、公開された checksum と照らすことが、この script が持てる唯一の
// 保証である。落とせなければ**入れずに止まる**ことまでを含めて確かめる。
func TestTheInstallScriptRefusesAnythingItCannotVerify(t *testing.T) {
	script := readInstallScript(t)
	for _, required := range []string{
		"checksums.txt",
		"sha256sum",
		"shasum",
		"does not match its published checksum",
		"cannot be verified",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("install.sh must verify what it downloads; %q is missing", required)
		}
	}
	// 検査より前に置いてしまっては意味がない。
	if strings.Index(script, "published checksum") > strings.Index(script, `mv "$work/sshc" "$target"`) {
		t.Error("install.sh installs before it verifies the checksum")
	}
}

// **利用者のものには触らない。**
//
// シェルの設定も、root の要る場所も、この script の持ち物ではない。PATH が
// 通っていないなら足す 1 行を綴るが、打つかどうかは向こうが決める——rustup が
// 訊いてから書くのは許されるが、**訊けない経路（パイプの向こう）では訊けない。**
func TestTheInstallScriptTouchesNothingItWasNotGiven(t *testing.T) {
	// **言うことと、やることを分ける。** 「sudo で入れ直してください」と綴るのは
	// 助言であり、`sudo` を実行するのとは違う——素朴に文字列を探すと、その二つが
	// 区別できない。注釈と、印字だけを行う helper の行を落としてから見る。
	var executable []string
	for _, line := range strings.Split(readInstallScript(t), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "note ") || strings.HasPrefix(trimmed, "say ") ||
			strings.HasPrefix(trimmed, "die ") || strings.HasPrefix(trimmed, "|| die ") {
			continue
		}
		executable = append(executable, trimmed)
	}
	body := strings.Join(executable, "\n")

	for _, forbidden := range []string{"sudo ", ">> $HOME", ">> $rc", ">>$rc"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("install.sh must not run %q itself", forbidden)
		}
	}
	// 助言そのものは残っていなければならない。**黙って諦めるのは、やるのと同じくらい悪い。**
	script := readInstallScript(t)
	if !strings.Contains(script, "is not on your PATH") {
		t.Error("install.sh must say so when the place it installed into is not on PATH")
	}
	if !strings.Contains(script, "$rc") {
		t.Error("install.sh must spell the line that puts it on PATH, even though it will not run it")
	}
}

// **落とす道具は二つある。** curl しか無い機械も wget しか無い機械もある。
func TestTheInstallScriptWorksWithEitherDownloader(t *testing.T) {
	script := readInstallScript(t)
	for _, tool := range []string{"curl", "wget"} {
		if !strings.Contains(script, "command -v "+tool) {
			t.Errorf("install.sh does not look for %s", tool)
		}
	}
}
