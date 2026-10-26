package buildcontract

import (
	"go/ast"
	"go/parser"
	"go/token"
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
	formula := readFormula(t)

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

// 画面（internal/ui/dist/app）はリポジトリに入っていない。formula が組み立てずに go build すると、
// brew で入れた sshc は画面の代わりに「画面が入っていない」と返すだけになる。brew の test do は
// version と status しか見ないので、そこでは気付けない。
func TestTheFormulaBuildsTheWebUIBeforeTheBinary(t *testing.T) {
	formula := readFormula(t)
	// CI とリリースと同じ Node 22 で組み立てる。Node が違うと組み立ての結果が変わる。
	if !strings.Contains(formula, `depends_on "node@22" => :build`) || !strings.Contains(formula, `ENV.prepend_path "PATH", Formula["node@22"].opt_bin`) {
		t.Fatal(`formula は node@22 で画面を組み立てていない（depends_on "node@22" => :build と、その bin を PATH の先頭に置く行が要る）`)
	}
	webBuild := regexp.MustCompile(`(?s)cd "web" do\s*\n\s*system "npm", "ci"[^\n]*\n\s*system "npm", "run", "build"\s*\n\s*end`).FindStringIndex(formula)
	if webBuild == nil {
		t.Fatal(`formula は web/ の中で npm ci と npm run build を走らせていない`)
	}
	goBuild := strings.Index(formula, `system "go", "build"`)
	if goBuild < 0 || goBuild < webBuild[1] {
		t.Fatal("formula は画面を組み立てる前に go build している")
	}
}

func TestTheFormulaInstallsShellCompletions(t *testing.T) {
	if !strings.Contains(readFormula(t), `generate_completions_from_executable(bin/"sshc", "completion")`) {
		t.Error("formula が sshc completion から bash/zsh/fish の補完を生成していない")
	}
}

// Homebrew 版も、リリースの成果物や make build と同じくタグの名前（v0.41.0）を名乗る。
//
// Homebrew の version はタグから v を除いた値（0.41.0）である。それをそのまま埋めていた
// あいだは、同じリリースでも Homebrew 版だけが `sshc 0.40.0` と名乗った。install.sh は
// 動いている engine のバージョンを文字列のまま比べるので、同じバージョンを入れても
// 「一致しない」と警告し、sshc update は「from 0.40.0 to v0.41.0」と表示した。
//
// brew の無いマシンでも走るよう、formula の文字列を確かめる。
func TestTheFormulaStampsTheTagNameLikeTheReleaseBuilds(t *testing.T) {
	formula := readFormula(t)
	for _, required := range []struct{ text, why string }{
		{`-X main.version=#{release_version}`, "go build が release_version を埋め込んでいない"},
		{`version.head? ? version.to_s : "v#{version}"`, "release_version がタグの v を付け直していない"},
		{`assert_match "sshc #{release_version}"`, "brew test が埋め込んだバージョンを確かめていない"},
	} {
		if !strings.Contains(formula, required.text) {
			t.Errorf("%s: formula に %q が無い", required.why, required.text)
		}
	}
}

// Homebrew 版も、About sshc にコミットとコミット日時を出す。
//
// formula はタグのソースアーカイブ（.git なし）からビルドするので、Go は vcs.revision・
// vcs.time を記録できない。v0.49.0 の About sshc は、コミット・コミット日時・未コミットの
// 変更が 3 行とも「不明」だった。release workflow がタグのコミットとその日時を formula へ
// 書き込み、formula が -X で渡す。HEAD のビルドは git の作業木から Go が記録する。
func TestTheFormulaPassesTheTaggedCommitThatTheArchiveLacks(t *testing.T) {
	formula := readFormula(t)
	for _, required := range []struct{ text, why string }{
		{"\n  SOURCE_REVISION = ", "release workflow の sed が書き換える SOURCE_REVISION の行が無い"},
		{"\n  SOURCE_COMMIT_TIME = ", "release workflow の sed が書き換える SOURCE_COMMIT_TIME の行が無い"},
		{`-X main.revision=#{SOURCE_REVISION}`, "go build がタグのコミットを渡していない"},
		{`-X main.commitTime=#{SOURCE_COMMIT_TIME}`, "go build がコミット日時を渡していない"},
		{`unless version.head?`, "HEAD のビルドにも直前のタグのコミットを渡してしまう"},
	} {
		if !strings.Contains(formula, required.text) {
			t.Errorf("%s: formula に %q が無い", required.why, required.text)
		}
	}

	_, workflow := readReleaseWorkflow(t)
	for _, required := range []string{
		`RELEASE_SHA: ${{ needs.verify-source.outputs.release-sha }}`,
		`-e "s#^  SOURCE_REVISION = .*#  SOURCE_REVISION = \"$RELEASE_SHA\".freeze#"`,
		`-e "s#^  SOURCE_COMMIT_TIME = .*#  SOURCE_COMMIT_TIME = \"$commit_time\".freeze#"`,
		`the commit was not written into the formula`,
		`the commit time was not written into the formula`,
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("release workflow がタグのコミットを formula へ書き込んでいない: %q が無い", required)
		}
	}
}

// formula が -X で名指す変数は、cmd/sshc に string の変数として実在しなければならない。
//
// go build は、存在しない変数への -X を何も言わずに無視する。名前を変えたり消したりすると、
// Homebrew 版だけがバージョンやコミットを失い、気付くのは利用者が About sshc を開いたときになる。
func TestTheFormulaStampsVariablesThatExist(t *testing.T) {
	stamped := regexp.MustCompile(`-X main\.(\w+)=`).FindAllStringSubmatch(readFormula(t), -1)
	if len(stamped) == 0 {
		t.Fatal("formula に -X main.<変数>= が無い")
	}
	declared := mainPackageStringVariables(t)
	for _, match := range stamped {
		if !declared[match[1]] {
			t.Errorf("formula は -X main.%s を渡すが、cmd/sshc にその名前の string の変数が無い", match[1])
		}
	}
}

// mainPackageStringVariables は、cmd/sshc の package レベルで string として宣言された変数の名前を返す。
func mainPackageStringVariables(t *testing.T) map[string]bool {
	t.Helper()
	files := token.NewFileSet()
	directory := filepath.Join("..", "..", "cmd", "sshc")
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	declared := map[string]bool{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(files, filepath.Join(directory, entry.Name()), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.VAR {
				continue
			}
			for _, spec := range general.Specs {
				value := spec.(*ast.ValueSpec)
				if !isStringVariable(value) {
					continue
				}
				for _, name := range value.Names {
					declared[name.Name] = true
				}
			}
		}
	}
	return declared
}

// isStringVariable は、-X が書き換えられる形（型が string、または string の定数で初期化）かを返す。
func isStringVariable(value *ast.ValueSpec) bool {
	if identifier, ok := value.Type.(*ast.Ident); ok {
		return identifier.Name == "string"
	}
	if value.Type != nil || len(value.Values) != len(value.Names) {
		return false
	}
	for _, initial := range value.Values {
		literal, ok := initial.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return false
		}
	}
	return true
}

// readFormula は、tap へ同期する formula の正本を読む。
func readFormula(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "packaging", "homebrew", "sshc.rb"))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
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
