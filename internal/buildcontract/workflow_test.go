package buildcontract

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestCIWorkflowProvidesNativeGoMatrices(t *testing.T) {
	document := readWorkflowDocument(t)
	if problems := validateNativeWorkflow(document); len(problems) != 0 {
		t.Fatalf("native workflow contract violations:\n- %s", strings.Join(problems, "\n- "))
	}
}

// VPN のコンテナイメージは利用者のマシンで作る。arm64 だけ壊れても amd64 の検査
// では気づけないので、両方のアーキテクチャで作る job を残す。
func TestCIWorkflowBuildsTheVPNImageOnBothArchitectures(t *testing.T) {
	document := readWorkflowDocument(t)
	job, ok := document.Jobs["vpn-image"]
	if !ok {
		t.Fatal("jobs.vpn-image is missing")
	}
	if job.Strategy == nil {
		t.Fatal("jobs.vpn-image has no matrix")
	}
	runners := map[string]bool{}
	for _, entry := range job.Strategy.Matrix.Include {
		runners[entry.OS] = true
	}
	for _, runner := range []string{"ubuntu-24.04", "ubuntu-24.04-arm"} {
		if !runners[runner] {
			t.Errorf("jobs.vpn-image does not run on %s", runner)
		}
	}
	// イメージの組み立てと接続の確かめは、関係する変更のときだけ走る。判定が答えを
	// 出せなかったとき（空のとき）は、'false' ではないので走る側に倒れる。
	const whenNeeded = "${{ steps.vpn.outputs.needed != 'false' }}"
	for _, want := range []struct{ name, run string }{
		{"Choose the image tag the engine uses", `echo "tag=sshc-vpn-test:$(scripts/ci/vpn-image-tag.sh)" >> "$GITHUB_OUTPUT"`},
		{"Connect through IKEv2/IPsec and L2TP/IPsec in the VPN container", "go test ./internal/vpn -run 'IKEv2|L2TP' -count=1 -timeout 15m"},
	} {
		step, found := namedStep(job, want.name)
		if !found || strings.TrimSpace(step.Run) != want.run || step.If != whenNeeded {
			t.Errorf("jobs.vpn-image must run %q as the step %q with if %s", want.run, want.name, whenNeeded)
		}
	}
	// イメージは main が保存したものをキャッシュから読み込み、無いときだけエンジンが付けるタグで
	// 組み立てる。Ubuntu の snapshot の障害で CI が落ちないためと、結合テストが組み立て直さない
	// ためである。
	restore, found := namedStep(job, "Restore the VPN container image")
	if !found || restore.If != whenNeeded || !strings.HasPrefix(restore.Uses, "actions/cache/restore@") ||
		restore.With["key"] != "vpn-image-${{ matrix.name }}-${{ steps.image.outputs.tag }}" {
		t.Error("jobs.vpn-image must restore the image saved under its tag before building it")
	}
	build, found := namedStep(job, "Build the VPN container image")
	if !found || strings.TrimSpace(build.Run) != `docker build --tag "$TAG" internal/vpn/container` ||
		build.If != "${{ steps.vpn.outputs.needed != 'false' && steps.image-cache.outputs.cache-hit != 'true' }}" {
		t.Error("jobs.vpn-image must build the image under the engine's tag only when the cache has none")
	}
	decide, found := namedStep(job, "Decide whether the VPN image check runs")
	if !found || !strings.Contains(decide.Run, "git diff --name-only HEAD^1 HEAD | scripts/ci/vpn-image-tests-needed.sh") {
		t.Error("jobs.vpn-image must decide from the change with scripts/ci/vpn-image-tests-needed.sh")
	}
}

// vpn-image-tests-needed.sh は、internal/vpn のテストが読み込むパッケージ、コンテナ、
// モジュールの定義、判定と workflow 自身のどれかが変わったときだけ true を返す。
func TestVPNImageTestsNeededScriptFollowsTheVPNTestDependencies(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the script runs on the Linux runner")
	}
	cases := []struct {
		name  string
		paths string
		want  string
	}{
		{"the web UI only", "web/src/App.tsx\npages/features/vpn.md\n", "false"},
		{"a package the VPN tests do not load", "internal/app/app.go\n", "false"},
		{"the VPN package", "web/src/App.tsx\ninternal/vpn/docker.go\n", "true"},
		{"a package the VPN tests load", "internal/storage/journal.go\n", "true"},
		{"the container", "internal/vpn/container/Dockerfile\n", "true"},
		{"the module file", "go.sum\n", "true"},
		{"the workflow itself", ".github/workflows/ci.yml\n", "true"},
		{"nothing given", "", "true"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			command := exec.Command("bash", filepath.Join("..", "..", "scripts", "ci", "vpn-image-tests-needed.sh"))
			command.Stdin = strings.NewReader(testCase.paths)
			output, err := command.Output()
			if err != nil {
				t.Fatalf("run script: %v", err)
			}
			if got := strings.TrimSpace(string(output)); got != testCase.want {
				t.Fatalf("VPN image check needed = %q, want %q", got, testCase.want)
			}
		})
	}
}

// 全E2E は実バイナリをブラウザから操作するため時間がかかる。独立jobにはせず、
// Makefile のローカル実行経路を維持する。主要画面のaxe検査だけはweb jobで走る。
func TestCIWorkflowLeavesTheFullEndToEndSuiteForLocalRuns(t *testing.T) {
	document := readWorkflowDocument(t)
	for _, id := range []string{"e2e", "e2e-windows"} {
		if _, present := document.Jobs[id]; present {
			t.Errorf("jobs.%s runs E2E in GitHub Actions; use make e2e locally", id)
		}
	}

	makefile, err := os.ReadFile(filepath.Join("..", "..", "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(makefile), "e2e: build\n\tnpm run e2e --prefix web") {
		t.Error("Makefile does not retain the local E2E target")
	}
}

func TestCIWorkflowRunsFocusedAccessibilitySuite(t *testing.T) {
	document := readWorkflowDocument(t)
	web, present := document.Jobs["web-checks"]
	if !present {
		t.Fatal("jobs.web-checks is missing")
	}

	wants := map[string]string{
		"Build accessibility test engine": "go build -o ../bin/sshc ../cmd/sshc",
		"Accessibility tests":             "e2e/accessibility.spec.ts",
	}
	for name, command := range wants {
		step, found := namedStep(web, name)
		if !found {
			t.Errorf("jobs.web-checks has no %q step", name)
			continue
		}
		if !strings.Contains(step.Run, command) {
			t.Errorf("jobs.web-checks step %q does not contain %q", name, command)
		}
	}

	// runner に入っている Google Chrome を使い、Chromium をダウンロードしない。ダウンロードは
	// 遅い日に止まって、検査ごと取り消されていた。
	if step, found := namedStep(web, "Accessibility tests"); found && step.Env["SSHC_E2E_BROWSER_CHANNEL"] != "chrome" {
		t.Error("jobs.web-checks Accessibility tests must run in the runner's Google Chrome (SSHC_E2E_BROWSER_CHANNEL: chrome)")
	}
	for _, step := range web.Steps {
		if strings.Contains(step.Run, "playwright install") {
			t.Errorf("jobs.web-checks step %q downloads a browser; use the runner's Google Chrome", step.Name)
		}
	}
}

// 画面の単体テストは、ファイルで分けて並べて流す（jobs.web-unit）。shard は 1 から数えて
// 抜けが無く、各 job は自分の番号と総数を vitest へ渡す。抜けがあれば、その分のテストが
// どこでも流れない。
func TestCIWorkflowSplitsTheWebUnitTestsWithoutLeavingAnyOut(t *testing.T) {
	document := readWorkflowDocument(t)
	unit, present := document.Jobs["web-unit"]
	if !present {
		t.Fatal("jobs.web-unit is missing")
	}
	if unit.Strategy == nil || unit.Strategy.FailFast == nil || *unit.Strategy.FailFast {
		t.Fatal("jobs.web-unit must be a matrix with fail-fast: false")
	}
	shards := unit.Strategy.Matrix.Shard
	if len(shards) < 2 {
		t.Fatalf("jobs.web-unit shards = %v, want at least two", shards)
	}
	for index, shard := range shards {
		if shard != index+1 {
			t.Fatalf("jobs.web-unit shards = %v, want 1 to %d without gaps", shards, len(shards))
		}
	}
	step, found := namedStep(unit, "Unit tests")
	if !found || strings.TrimSpace(step.Run) != "npm test -- --shard=${{ matrix.shard }}/${{ strategy.job-total }}" {
		t.Error("jobs.web-unit must run npm test with its own shard out of the matrix total")
	}
}

// 必須の検査「Web」は、分けた画面の job をまとめる。どれかが成功以外で終われば失敗し、
// 依存先の失敗で飛ばされないよう !cancelled() で必ず走る。
func TestCIWorkflowReportsWebOnlyWhenEveryWebJobSucceeded(t *testing.T) {
	document := readWorkflowDocument(t)
	web, present := document.Jobs["web"]
	if !present {
		t.Fatal("jobs.web is missing")
	}
	if web.Name != "Web" {
		t.Errorf("jobs.web name = %q, want the required check name Web", web.Name)
	}
	if !slices.Equal(web.Needs, []string{"web-checks", "web-unit"}) {
		t.Errorf("jobs.web needs = %v, want [web-checks web-unit]", web.Needs)
	}
	if web.If != "${{ !cancelled() }}" {
		t.Errorf("jobs.web if = %q, want ${{ !cancelled() }} so that a failed web job fails Web instead of skipping it", web.If)
	}
	step, found := namedStep(web, "Require every web job to succeed")
	if !found || step.Env["RESULTS"] != "${{ join(needs.*.result, ' ') }}" ||
		!strings.Contains(step.Run, `if [ "$result" != success ]; then`) || !strings.Contains(step.Run, "exit 1") {
		t.Error("jobs.web must fail unless every needed web job succeeded")
	}
}

// withoutYAMLComments は、注釈の行を落とす。
//
// 説明ではなく、書いてあることを見る。注釈の中の語で落ちる検査は、いずれ
// 注釈を消すことで直される。残すべきものの方が先に消える。
func withoutYAMLComments(text string) string {
	kept := make([]string, 0, 32)
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// jobSection は、ジョブの見出しから次の見出しまでを返す。
func jobSection(text, from, to string) string {
	start := strings.Index(text, from)
	if start < 0 {
		return ""
	}
	rest := text[start:]
	if end := strings.Index(rest, to); end >= 0 {
		return rest[:end]
	}
	return rest
}

// Windows の go test は、ログを残すために出力を Tee-Object へ流すので、終了コードを
// 自分で返す。そこを exit 0 に変えて失敗を隠すと契約が落ちることを確かめる。
func TestCIWorkflowRejectsWindowsTestStepsThatHideFailures(t *testing.T) {
	path := workflowPath()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	for _, exitLine := range []string{"exit $raceExit"} {
		t.Run(exitLine, func(t *testing.T) {
			if !strings.Contains(string(source), exitLine) {
				t.Fatalf("workflow does not contain %q", exitLine)
			}
			mutated := strings.Replace(string(source), exitLine, "exit 0", 1)
			document, err := decodeWorkflowDocument([]byte(mutated))
			if err != nil {
				t.Fatalf("decode mutated workflow: %v", err)
			}
			if problems := validateNativeWorkflow(document); len(problems) == 0 {
				t.Fatal("a Windows test step that always exits 0 unexpectedly satisfies the workflow contract")
			}
		})
	}
}

func TestNativeGofmtScriptReportsOnlyExactTrackedUnformattedPaths(t *testing.T) {
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}

	fixture := t.TempDir()
	runCommand(t, fixture, "git", "init", "--quiet")
	writeFixture(t, fixture, "formatted.go", "package fixture\n\nfunc Formatted() {}\n")
	tracked := []string{"-dash.go", "space name.go", "日本語.go"}
	if runtime.GOOS != "windows" {
		// Win32 はファイル名の制御文字 1〜31 を拒否するため、Windows fixture では表現可能な
		// 不正名をすべて検査し、Unix では改行を含む名前により、行ではなく NUL が転送境界で
		// あることを検証する。
		tracked = append(tracked, "line\nbreak.go")
	}
	for _, name := range tracked {
		writeFixture(t, fixture, name, "package fixture\nfunc Unformatted( ) {}\n")
	}
	writeFixture(t, fixture, "ignored 日本語.go", "package fixture\nfunc Ignored( ) {}\n")
	addArgs := append([]string{"add", "--", "formatted.go"}, tracked...)
	runCommand(t, fixture, "git", addArgs...)

	rawPaths := runCommandOutput(t, fixture, "git", "ls-files", "-z", "--", "*.go")
	// Git は index をバイト順に並べるため formatted.go は line/space より前になる。
	// 行単位または引用済み出力を検出できるよう、手計算した値を明示する。
	var wantRawPaths string
	if runtime.GOOS != "windows" {
		wantRawPaths = "-dash.go\x00formatted.go\x00line\nbreak.go\x00space name.go\x00日本語.go\x00"
	} else {
		wantRawPaths = "-dash.go\x00formatted.go\x00space name.go\x00日本語.go\x00"
	}
	if string(rawPaths) != wantRawPaths {
		t.Fatalf("git NUL path fixture = %q, want %q", rawPaths, wantRawPaths)
	}

	command, args := gofmtScriptCommand(repository)
	cmd := exec.Command(command, args...)
	cmd.Dir = fixture
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("formatter script succeeded with an unformatted tracked file; output:\n%s", output)
	}
	normalized := strings.ReplaceAll(string(output), "\r\n", "\n")
	wantPaths := []string{"-dash.go"}
	if runtime.GOOS != "windows" {
		wantPaths = append(wantPaths, "line\nbreak.go")
	}
	wantPaths = append(wantPaths, "space name.go", "日本語.go")
	want := "These files are not gofmt-formatted. Run: gofmt -w <path>.\n" + strings.Join(wantPaths, "\n") + "\n"
	if normalized != want {
		t.Fatalf("formatter diagnostics = %q, want exact %q", normalized, want)
	}

	for _, name := range tracked {
		writeFixture(t, fixture, name, "package fixture\n\nfunc Unformatted() {}\n")
	}
	cmd = exec.Command(command, args...)
	cmd.Dir = fixture
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("formatter script rejected formatted tracked files: %v\n%s", err, output)
	}
	if len(output) != 0 {
		t.Fatalf("formatter script emitted output on success: %q", output)
	}
}

func TestNativeGofmtScriptChecksMorePathsThanOneWindowsCommandLineHolds(t *testing.T) {
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	fixture := t.TempDir()
	runCommand(t, fixture, "git", "init", "--quiet")

	// CreateProcessが受け付けるコマンドラインの最大文字数。
	const windowsCommandLineCharacters = 32767
	// この上限を超える長さのパスを作る。名前の長さは、一時ディレクトリと合わせても
	// WindowsのMAX_PATH（260文字）に収まるように抑える。
	const pathCount = 450
	namePrefix := strings.Repeat("p", 80)
	names := make([]string, pathCount)
	totalCharacters := 0
	for index := range names {
		names[index] = fmt.Sprintf("%s%03d.go", namePrefix, index)
		totalCharacters += len(names[index]) + 1
		writeFixture(t, fixture, names[index], "package fixture\n")
	}
	if totalCharacters <= windowsCommandLineCharacters {
		t.Fatalf("fixture paths use %d characters, which fit on one Windows command line", totalCharacters)
	}
	// 最初と最後のファイルだけ崩し、分けて渡したどの回の結果も報告されることを確かめる。
	unformattedNames := []string{names[0], names[pathCount-1]}
	for _, name := range unformattedNames {
		writeFixture(t, fixture, name, "package fixture\nfunc Unformatted( ) {}\n")
	}
	runCommand(t, fixture, "git", "add", "--all")

	command, args := gofmtScriptCommand(repository)
	cmd := exec.Command(command, args...)
	cmd.Dir = fixture
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("formatter script succeeded with unformatted tracked files; output:\n%s", output)
	}
	normalized := strings.ReplaceAll(string(output), "\r\n", "\n")
	want := "These files are not gofmt-formatted. Run: gofmt -w <path>.\n" + strings.Join(unformattedNames, "\n") + "\n"
	if normalized != want {
		t.Fatalf("formatter diagnostics = %q, want exact %q", normalized, want)
	}
}

// gofmtScriptCommand は、このOSのCIが使うgofmtの確認スクリプトを起動するコマンドを返す。
func gofmtScriptCommand(repository string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "pwsh", []string{"-NoProfile", "-File", filepath.Join(repository, "scripts", "ci", "check-gofmt.ps1")}
	}
	return "sh", []string{filepath.Join(repository, "scripts", "ci", "check-gofmt.sh")}
}

func TestWindowsGofmtScriptUsesRawNULTerminatedGitOutput(t *testing.T) {
	path := filepath.Join("..", "..", "scripts", "ci", "check-gofmt.ps1")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	script := string(source)
	for _, required := range []string{
		"[Diagnostics.ProcessStartInfo]::new()",
		"RedirectStandardOutput = $true",
		"UseShellExecute = $false",
		"ArgumentList.Add(\"ls-files\")",
		"ArgumentList.Add(\"-z\")",
		"ArgumentList.Add(\"--\")",
		"ArgumentList.Add(\"*.go\")",
		"StandardOutput.BaseStream.CopyTo(",
		"WaitForExit()",
		"$gitProcess.ExitCode",
		"[Text.UTF8Encoding]::new($false, $true)",
		"Split([char]0, [StringSplitOptions]::RemoveEmptyEntries)",
		"gofmt -l -- @batchPaths",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("Windows formatter lacks raw NUL path transport fragment %q", required)
		}
	}
	for _, forbidden := range []string{
		"& git ls-files",
		"StandardOutput.ReadToEnd()",
	} {
		if strings.Contains(script, forbidden) {
			t.Errorf("Windows formatter uses lossy string/line transport %q", forbidden)
		}
	}
}

func readWorkflowDocument(t *testing.T) workflowDocument {
	t.Helper()
	document, _ := readWorkflowFile(t, workflowPath())
	return document
}

func workflowPath() string {
	return filepath.Join(workflowsDirectory(), "ci.yml")
}

func validateNativeWorkflow(document workflowDocument) []string {
	var problems []string
	goJob, ok := document.Jobs["go"]
	if !ok {
		problems = append(problems, "jobs.go is missing")
	} else {
		problems = append(problems, validateNativeMatrix("jobs.go", goJob)...)
		problems = append(problems, validateGoSteps(goJob)...)
		problems = append(problems, validatePinnedSetup(goJob, "actions/checkout")...)
		problems = append(problems, validatePinnedSetup(goJob, "actions/setup-go")...)
		problems = append(problems, validateGoBuildCache(goJob)...)
		problems = append(problems, validateSetupOrder("jobs.go", goJob, "actions/setup-go")...)
	}

	if _, ok := document.Jobs["macos"]; ok {
		problems = append(problems, "the old jobs.macos duplicate must be folded into jobs.go")
	}
	for _, id := range []string{"web", "web-checks", "generated", "integration", "security", "android", "deadcode"} {
		job, ok := document.Jobs[id]
		if !ok {
			problems = append(problems, "single-instance job "+id+" is missing")
			continue
		}
		if job.Strategy != nil && len(job.Strategy.Matrix.Include) != 0 {
			problems = append(problems, "jobs."+id+" must remain single-instance")
		}
	}
	return problems
}

// validateNativeTestsGate は、画面だけの PR で Windows と macOS の Go のテストを飛ばす仕組みを
// 確かめる。必須の検査（Go native (Windows) など）は、job ごと消すと PR が「待ち」のまま
// マージできなくなる。そこで job は残し、jobs.changes の答えで手順だけを飛ばす。Linux はいつも流す。
func validateNativeTestsGate(job workflowJob) []string {
	var problems []string
	// jobs.ui は、Go のテストが読む組み立てた画面を渡す（embedded_ui_workflow_test.go）。
	if !slices.Equal(job.Needs, []string{"changes", "ui"}) {
		problems = append(problems, "jobs.go must need jobs.changes to know whether the native Go tests run, and jobs.ui for the built web UI")
	}
	if job.If != "${{ !cancelled() }}" {
		problems = append(problems, "jobs.go must run even when jobs.changes fails, with if: ${{ !cancelled() }}")
	}
	if job.Env["NATIVE_TESTS"] != nativeTestsExpression {
		problems = append(problems, fmt.Sprintf("jobs.go env.NATIVE_TESTS = %q, want %q", job.Env["NATIVE_TESTS"], nativeTestsExpression))
	}
	for _, step := range job.Steps {
		if step.If == "" {
			problems = append(problems, fmt.Sprintf("jobs.go step %q must say whether it runs for a web-only change", stepLabel(step)))
		}
	}
	return problems
}

func stepLabel(step workflowStep) string {
	if step.Name != "" {
		return step.Name
	}
	return step.Uses
}

func validateNativeMatrix(id string, job workflowJob) []string {
	var problems []string
	if job.RunsOn != "${{ matrix.os }}" {
		problems = append(problems, id+" runs-on must be ${{ matrix.os }}")
	}
	if job.Strategy == nil {
		return append(problems, id+" strategy is missing")
	}
	if job.Strategy.FailFast == nil || *job.Strategy.FailFast {
		problems = append(problems, id+" must explicitly set fail-fast: false")
	}

	want := map[string]string{
		"ubuntu-24.04": "Linux",
		"macos-15":     "macOS",
		"windows-2025": "Windows",
	}
	got := make(map[string]string, len(job.Strategy.Matrix.Include))
	for _, entry := range job.Strategy.Matrix.Include {
		if _, duplicate := got[entry.OS]; duplicate {
			problems = append(problems, id+" has duplicate matrix OS "+entry.OS)
		}
		got[entry.OS] = entry.Name
	}
	if len(got) != len(want) {
		problems = append(problems, fmt.Sprintf("%s matrix OS count = %d, want %d", id, len(got), len(want)))
	}
	for osName, displayName := range want {
		if got[osName] != displayName {
			problems = append(problems, fmt.Sprintf("%s matrix entry %s name = %q, want %q", id, osName, got[osName], displayName))
		}
	}
	for osName := range got {
		if _, ok := want[osName]; !ok {
			problems = append(problems, id+" has unsupported matrix OS "+osName)
		}
	}
	return problems
}

// nativeTestsGate は、jobs.go の各手順の条件の終わりに付く「Go に関わる変更があるときだけ」で
// ある。NATIVE_TESTS は、Linux ではいつも、Windows と macOS では jobs.changes が要ると決めたときに
// true になる。
const nativeTestsGate = " && env.NATIVE_TESTS == 'true'"

// nativeTestsExpression は、jobs.go の env.NATIVE_TESTS に書く式である。
// jobs.changes が失敗して答えが空のときも流れるよう、'true' ではなく 'false' と比べる。
const nativeTestsExpression = "${{ matrix.name == 'Linux' || needs.changes.outputs.native != 'false' }}"

// gated は、手順の条件 "${{ ... }}" に nativeTestsGate を足す。
func gated(condition string) string {
	return strings.TrimSuffix(condition, " }}") + nativeTestsGate + " }}"
}

func validateGoSteps(job workflowJob) []string {
	var problems []string
	problems = append(problems, validateSeparatedRunShells("jobs.go", job)...)
	problems = append(problems, validateNativeTestsGate(job)...)
	for _, required := range []runContract{
		{run: "scripts/ci/check-gofmt.sh", condition: gated("${{ runner.os != 'Windows' }}"), shell: "bash"},
		{run: "./scripts/ci/check-gofmt.ps1", condition: gated("${{ runner.os == 'Windows' }}"), shell: "pwsh"},
		{run: "go vet ./...", condition: gated("${{ runner.os != 'Windows' }}"), shell: "bash"},
		{run: "go vet ./...", condition: gated("${{ runner.os == 'Windows' }}"), shell: "pwsh"},
		{run: "go build ./...", condition: gated("${{ runner.os != 'Windows' }}"), shell: "bash"},
		{run: "go build ./...", condition: gated("${{ runner.os == 'Windows' }}"), shell: "pwsh"},
		// テストは OS ごとに 1 回、race を付けて流す。PR でも main への push でも同じ。
		{run: "go test -count=1 -race -tags sshc_test_kdf ./...", condition: gated("${{ runner.os != 'Windows' }}"), shell: "bash"},
		{run: windowsFastTempScript, condition: gated("${{ runner.os == 'Windows' }}"), shell: "pwsh"},
	} {
		if !hasRunContract(job, required) {
			problems = append(problems, fmt.Sprintf("jobs.go lacks run=%q if=%q shell=%q", required.run, required.condition, required.shell))
		}
	}

	const windowsTest = "go test -race (Windows)"
	problems = append(problems, validateWindowsLoggedTest(job, windowsLoggedTest{
		name:         windowsTest,
		condition:    gated("${{ runner.os == 'Windows' }}"),
		command:      "& go test -v -count=1 -race -tags sshc_test_kdf ./... 2>&1 | Tee-Object -FilePath",
		exitVariable: "$raceExit",
	})...)
	if !runsBefore(job, windowsFastTempScript, windowsTest) {
		problems = append(problems, fmt.Sprintf("jobs.go must run %s before the step %q, or the Windows tests write to the slow system drive", windowsFastTempScript, windowsTest))
	}
	return problems
}

// windowsFastTempScript は、Windows の TEMP を OS のディスクから作業用のディスクへ移す。
// 移さないと、非公開の書き込みのたびのディスクへの書き出しで、Windows の go test が
// 数倍遅くなる（scripts/ci/windows-fast-temp.ps1）。
const windowsFastTempScript = "./scripts/ci/windows-fast-temp.ps1"

// runsBefore は、script を走らせる手順が、name の手順より前にあるかを返す。
func runsBefore(job workflowJob, script, name string) bool {
	scriptIndex, nameIndex := -1, -1
	for index, step := range job.Steps {
		if strings.TrimSpace(step.Run) == script && scriptIndex < 0 {
			scriptIndex = index
		}
		if step.Name == name {
			nameIndex = index
		}
	}
	return scriptIndex >= 0 && nameIndex >= 0 && scriptIndex < nameIndex
}

// windowsLoggedTest は、出力をログへ残しながら go test を走らせる Windows の step の
// 期待値である。
type windowsLoggedTest struct {
	name         string
	condition    string
	command      string
	exitVariable string
}

// validateWindowsLoggedTest は、step が go test の終了コードをそのまま返すことを
// 確かめる。Tee-Object を通すと PowerShell は失敗を自動では伝えないので、終了
// コードを変数に取って exit で返す形だけを認める。
func validateWindowsLoggedTest(job workflowJob, want windowsLoggedTest) []string {
	step, ok := namedStep(job, want.name)
	if !ok {
		return []string{"jobs.go lacks the step " + want.name}
	}
	var problems []string
	if step.If != want.condition || step.Shell != "pwsh" {
		problems = append(problems, fmt.Sprintf("step %q must run with if=%q and shell pwsh", want.name, want.condition))
	}
	if step.ContinueOnError != nil {
		problems = append(problems, fmt.Sprintf("step %q must not use continue-on-error", want.name))
	}
	for _, fragment := range []string{
		"$PSNativeCommandUseErrorActionPreference = $false",
		want.command,
		want.exitVariable + " = $LASTEXITCODE",
		"exit " + want.exitVariable,
	} {
		if !strings.Contains(step.Run, fragment) {
			problems = append(problems, fmt.Sprintf("step %q lacks fragment %q", want.name, fragment))
		}
	}
	if strings.Contains(step.Run, "exit 0") {
		problems = append(problems, fmt.Sprintf("step %q must not turn a failure into exit 0", want.name))
	}
	return problems
}

func validatePinnedSetup(job workflowJob, action string) []string {
	var matches []workflowStep
	for _, step := range job.Steps {
		if strings.HasPrefix(step.Uses, action+"@") {
			matches = append(matches, step)
		}
	}
	if len(matches) != 1 {
		return []string{fmt.Sprintf("job %q has %d %s steps, want 1", job.Name, len(matches), action)}
	}
	step := matches[0]
	var problems []string
	if action == "actions/setup-go" {
		if fmt.Sprint(step.With["go-version-file"]) != "go.mod" || fmt.Sprint(step.With["cache"]) != "false" {
			problems = append(problems, "native Go setup must use go.mod and leave caching to the Go build cache steps (cache: false)")
		}
	}
	return problems
}

// validateGoBuildCache は、jobs.go のビルドのキャッシュを確かめる。setup-go のキャッシュは
// go.sum だけで鍵を作るので、依存が変わらない限り作り直されず、そのあとに変わったパッケージを
// 毎回コンパイルし直す。main のコミットごとに保存し、PR と push は最新のものから始める。
func validateGoBuildCache(job workflowJob) []string {
	const key = "go-${{ matrix.name }}-${{ hashFiles('go.sum') }}-${{ github.sha }}"
	var problems []string
	restore, found := namedStep(job, "Restore the Go build cache")
	if !found || !strings.HasPrefix(restore.Uses, "actions/cache/restore@") || fmt.Sprint(restore.With["key"]) != key ||
		!strings.Contains(fmt.Sprint(restore.With["restore-keys"]), "go-${{ matrix.name }}-${{ hashFiles('go.sum') }}-\n") {
		problems = append(problems, "jobs.go must restore the newest Go build cache of its OS with the key "+key)
	}
	save, found := namedStep(job, "Save the Go build cache")
	if !found || !strings.HasPrefix(save.Uses, "actions/cache/save@") || fmt.Sprint(save.With["key"]) != key ||
		save.If != gated("${{ github.event_name == 'push' && github.ref == 'refs/heads/main' }}") {
		problems = append(problems, "jobs.go must save the Go build cache only from pushes to main, with the key "+key)
	}
	return problems
}

// mainPushOnly は、main への push のときだけ走る手順（キャッシュの保存の前の整理）が
// OS の条件に足す条件である。OS の分け方は、ほかの手順と同じ決まりで確かめる。
const mainPushOnly = " && github.event_name == 'push' && github.ref == 'refs/heads/main'"

func validateSeparatedRunShells(id string, job workflowJob) []string {
	var problems []string
	for _, step := range job.Steps {
		if step.Run == "" {
			continue
		}
		switch strings.Replace(step.If, mainPushOnly, "", 1) {
		// 画面だけの変更で Go のテストを飛ばしたことを知らせる echo だけは、どの OS でも bash で走る。
		case "${{ env.NATIVE_TESTS != 'true' }}":
			if step.Shell != "bash" || !strings.HasPrefix(strings.TrimSpace(step.Run), "echo ") {
				problems = append(problems, fmt.Sprintf("%s step %q must only echo, in bash", id, step.Name))
			}
		// Linux だけの手順は、どのランナーでも結果の同じ検査（別の OS 向けのビルドなど）を1回で済ませる。
		case gated("${{ runner.os != 'Windows' }}"), gated("${{ runner.os == 'Linux' }}"):
			if step.Shell != "bash" {
				problems = append(problems, fmt.Sprintf("%s step %q must use bash for its Unix-only command", id, step.Name))
			}
		case gated("${{ runner.os == 'Windows' }}"):
			// The public installer explicitly supports the inbox Windows PowerShell
			// 5.1. Its syntax check and the installer smoke (the one release.yml runs)
			// must use that executable; every build/test step continues to use the
			// pinned pwsh runtime on the runner.
			if step.Name == "Windows PowerShell installer syntax" || step.Name == "Release installer smoke (Windows)" {
				if step.Shell != "powershell" {
					problems = append(problems, fmt.Sprintf("%s step %q must use Windows PowerShell 5.1", id, step.Name))
				}
			} else if step.Shell != "pwsh" {
				problems = append(problems, fmt.Sprintf("%s step %q must use pwsh for its Windows-only command", id, step.Name))
			}
		default:
			problems = append(problems, fmt.Sprintf("%s step %q must be split into an explicit Unix or Windows command", id, step.Name))
		}
		if step.ContinueOnError != nil {
			problems = append(problems, fmt.Sprintf("%s step %q must not use continue-on-error", id, step.Name))
		}
	}
	return problems
}

func validateSetupOrder(id string, job workflowJob, setupAction string) []string {
	checkoutIndex := -1
	setupIndex := -1
	firstRunIndex := -1
	for index, step := range job.Steps {
		if strings.HasPrefix(step.Uses, "actions/checkout@") {
			checkoutIndex = index
		}
		if strings.HasPrefix(step.Uses, setupAction+"@") {
			setupIndex = index
		}
		if firstRunIndex < 0 && step.Run != "" {
			firstRunIndex = index
		}
	}
	if checkoutIndex < 0 || setupIndex < 0 || firstRunIndex < 0 || checkoutIndex >= setupIndex || setupIndex >= firstRunIndex {
		return []string{fmt.Sprintf("%s must checkout, set up its toolchain, then run commands", id)}
	}
	return nil
}

type runContract struct {
	run       string
	condition string
	shell     string
}

func hasRunContract(job workflowJob, want runContract) bool {
	for _, step := range job.Steps {
		if strings.TrimSpace(step.Run) == want.run && step.If == want.condition && step.Shell == want.shell {
			return true
		}
	}
	return false
}

func namedStep(job workflowJob, name string) (workflowStep, bool) {
	for _, step := range job.Steps {
		if step.Name == name {
			return step, true
		}
	}
	return workflowStep{}, false
}

func writeFixture(t *testing.T, directory, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o600); err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
	}
}

func runCommand(t *testing.T, directory, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = directory
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run %s: %v\n%s", name, err, output)
	}
}

func runCommandOutput(t *testing.T, directory, name string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = directory
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("run %s: %v", name, err)
	}
	return output
}

// jobs.changes は、PR の merge commit と 1 つ目の親（base）との差分を native-tests-needed.sh に
// 渡し、Windows と macOS の Go のテストが要るかを jobs.go へ渡す。
func TestCIWorkflowDecidesTheNativeGoTestsFromThePullRequestDiff(t *testing.T) {
	document := readWorkflowDocument(t)
	job, ok := document.Jobs["changes"]
	if !ok {
		t.Fatal("jobs.changes is missing")
	}
	step, found := namedStep(job, "Decide whether the Windows and macOS Go tests run")
	if !found {
		t.Fatal("jobs.changes has no step deciding the native Go tests")
	}
	for _, fragment := range []string{
		`if [ "$EVENT_NAME" != "pull_request" ]; then`,
		`git diff --name-only HEAD^1 HEAD | scripts/ci/native-tests-needed.sh`,
		`>> "$GITHUB_OUTPUT"`,
	} {
		if !strings.Contains(step.Run, fragment) {
			t.Errorf("the deciding step lacks %q", fragment)
		}
	}
	for _, checkout := range job.Steps {
		if strings.HasPrefix(checkout.Uses, "actions/checkout@") && fmt.Sprint(checkout.With["fetch-depth"]) != "2" {
			t.Errorf("jobs.changes checkout fetch-depth = %v, want 2 to reach the merge commit's base", checkout.With["fetch-depth"])
		}
	}
}

// native-tests-needed.sh は、画面・文書・組み立てた画面のファイル・確認用の画像・Markdown だけの
// 変更なら false、それ以外が 1 つでもあるか、パスを受け取れなかったら true を返す。
func TestNativeTestsNeededScriptSkipsOnlyWebAndDocumentChanges(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the script runs on the Linux runner")
	}
	cases := []struct {
		name  string
		paths string
		want  string
	}{
		{"web and documents only", "web/src/App.tsx\npages/features/menu.md\nartifacts/x.png\nREADME.md\n", "false"},
		{"a Go file among them", "web/src/App.tsx\ninternal/app/app.go\n", "true"},
		{"the workflow itself", ".github/workflows/ci.yml\n", "true"},
		{"the module file", "go.mod\n", "true"},
		{"nothing given", "", "true"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			command := exec.Command("bash", filepath.Join("..", "..", "scripts", "ci", "native-tests-needed.sh"))
			command.Stdin = strings.NewReader(testCase.paths)
			output, err := command.Output()
			if err != nil {
				t.Fatalf("run script: %v", err)
			}
			if got := strings.TrimSpace(string(output)); got != testCase.want {
				t.Fatalf("native tests needed = %q, want %q", got, testCase.want)
			}
		})
	}
}
