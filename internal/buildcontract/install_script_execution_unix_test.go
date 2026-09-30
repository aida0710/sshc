//go:build !windows

package buildcontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// installScriptRun は、install.shを1回実行した結果である。
type installScriptRun struct {
	output           string
	err              error
	installDirectory string
	// assetDigest は、配布物として渡した本文のSHA-256である。
	assetDigest string
	// gitHubCLICalls は、偽のghが受け取った引数を1回1行で並べたものである。
	gitHubCLICalls string
}

// installScriptSetup は、install.shを1回実行するときの条件である。
type installScriptSetup struct {
	// assetBody は、配布物のsshcとして渡す実行可能なスクリプトである。
	assetBody string
	// gitHubCLI は、PATHに置くghの偽物の本文である。空ならghの無いマシンになる。
	gitHubCLI string
	// environment は、既定の環境変数を上書きする"NAME=value"である。
	environment []string
	// runningEngineVersion は、PATHに置くsshcの偽物が`sshc status --json`で答える、動いている
	// engineのバージョンである。空ならsshcの無いマシンになる。
	runningEngineVersion string
}

// signedInGitHubCLI は、github.comにログインしたghの偽物である。attestation verifyは、
// SSHC_TEST_ATTESTATION_ERRORが空なら成功し、空でなければそれを表示して失敗する。
// --helpは本物と同じく、検証の結果によらず成功する。
const signedInGitHubCLI = `#!/bin/sh
printf '%s\n' "$*" >> "$SSHC_TEST_GH_CALLS"
case "$*" in
  "attestation verify --help") exit 0 ;;
esac
case "$1 $2" in
  "auth status") exit 0 ;;
  "attestation verify")
    if [ -n "${SSHC_TEST_ATTESTATION_ERROR:-}" ]; then
      printf '%s\n' "$SSHC_TEST_ATTESTATION_ERROR" >&2
      exit 1
    fi
    ;;
  *) exit 1 ;;
esac
`

// signedOutGitHubCLI は、どのホストにもログインしていないghの偽物である。本物と同じく、
// --helpはログインしていなくても表示できる。
const signedOutGitHubCLI = `#!/bin/sh
printf '%s\n' "$*" >> "$SSHC_TEST_GH_CALLS"
case "$*" in
  "attestation verify --help") exit 0 ;;
esac
case "$1 $2" in
  "auth status") echo "You are not logged into any GitHub hosts." >&2; exit 1 ;;
  *) echo "To get started with GitHub CLI, please run: gh auth login" >&2; exit 4 ;;
esac
`

// gitHubCLIWithoutAttestation は、gh attestationが入る前のghの偽物である。github.comには
// ログインしている。Ubuntu 24.04のaptが入れるgh 2.45.0と同じく、attestationを知らない
// コマンドとして扱い、--helpを付けても失敗する。
const gitHubCLIWithoutAttestation = `#!/bin/sh
printf '%s\n' "$*" >> "$SSHC_TEST_GH_CALLS"
case "$1 $2" in
  "auth status") exit 0 ;;
  attestation*) printf 'unknown command "%s" for "gh"\n' "$1" >&2; exit 1 ;;
  *) exit 1 ;;
esac
`

// attestationVerifications は、偽のghが受け取った引数のうち、成果物の検証を頼んだ行を返す。
// 検証できるghかを確かめる`attestation verify --help`は含めない。
func attestationVerifications(gitHubCLICalls string) []string {
	var verifications []string
	for _, call := range strings.Split(gitHubCLICalls, "\n") {
		if strings.HasPrefix(call, "attestation verify ") && call != "attestation verify --help" {
			verifications = append(verifications, call)
		}
	}
	return verifications
}

// installScriptSystemTools は、install.shと偽物のコマンドがPATHから探す本物のコマンドである。
// PATHにはこれだけを置き、ホストにあるghやsshcをinstall.shに見せない。
var installScriptSystemTools = []string{
	"cat", "chmod", "cp", "cut", "grep", "id", "mkdir", "mktemp", "readlink", "rm", "sed", "sha256sum", "shasum",
}

// linkInstallScriptSystemTools は、installScriptSystemToolsをdirectoryへsymlinkで並べる。
// sha256sumとshasumは、install.shがどちらか一方で足りるので、無いものは飛ばす。
func linkInstallScriptSystemTools(t *testing.T, directory string) {
	t.Helper()
	for _, tool := range installScriptSystemTools {
		path, err := exec.LookPath(tool)
		if err != nil {
			if tool == "sha256sum" || tool == "shasum" {
				continue
			}
			t.Skipf("install.sh needs %s", tool)
		}
		if err := os.Symlink(path, filepath.Join(directory, tool)); err != nil {
			t.Fatal(err)
		}
	}
}

// installScriptCommands は、install.shが外の世界を見るコマンドの偽物である。unameは
// SSHC_TEST_UNAME_SYSTEMとSSHC_TEST_UNAME_MACHINEを返し、sysctlはSSHC_TEST_ARM64_CPUが
// 空でなければhw.optional.arm64としてそれを返す（空ならそのOIDのないIntelのMac）。
// mvは、移動先がSSHC_TEST_FAIL_MV_TOのときだけ失敗し、ほかは本物のmvに任せる。
var installScriptCommands = map[string]string{
	"mv": `#!/bin/sh
for destination; do :; done
if [ -n "${SSHC_TEST_FAIL_MV_TO:-}" ] && [ "$destination" = "$SSHC_TEST_FAIL_MV_TO" ]; then
  echo "mv: simulated failure" >&2
  exit 1
fi
exec /bin/mv "$@"
`,
	"curl": `#!/bin/sh
set -eu
url=""
output=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) output=$2; shift 2 ;;
    http*) url=$1; shift ;;
    *) shift ;;
  esac
done
case "$url" in
  */checksums.txt) cp "$SSHC_TEST_FIXTURES/checksums.txt" "$output" ;;
  */sshc-*) cp "$SSHC_TEST_FIXTURES/${url##*/}" "$output" ;;
  *) exit 22 ;;
esac
`,
	"uname": `#!/bin/sh
case "$1" in
  -s) printf '%s\n' "$SSHC_TEST_UNAME_SYSTEM" ;;
  -m) printf '%s\n' "$SSHC_TEST_UNAME_MACHINE" ;;
  *) exit 1 ;;
esac
`,
	"sysctl": `#!/bin/sh
if [ "$*" = "-n hw.optional.arm64" ] && [ -n "${SSHC_TEST_ARM64_CPU:-}" ]; then
  printf '%s\n' "$SSHC_TEST_ARM64_CPU"
  exit 0
fi
printf 'sysctl: unknown oid %s\n' "$2" >&2
exit 1
`,
}

// installScriptAssets は、install.shが選びうるmacOSとLinuxの成果物の名前である。偽のunameが
// どれを名乗らせても取得できるよう、すべて同じ本文で用意する。
var installScriptAssets = []string{"sshc-linux-amd64", "sshc-linux-arm64", "sshc-darwin-amd64", "sshc-darwin-arm64"}

// defaultFixturePlatform は、偽のunameが既定で名乗るLinuxのx86_64に対応する成果物の対象である。
const defaultFixturePlatform = "linux/amd64"

// runningEngineCommand は、versionのengineが動いているマシンのsshcの偽物である。
// `sshc status --json`にだけ、本物と同じ成功の封筒で答え、ほかの引数では失敗する。
// install.shは、その result の version を読んで、動いている engine とのバージョンの違いを知らせる。
func runningEngineCommand(version string) string {
	return `#!/bin/sh
[ "$1 $2" = "status --json" ] || exit 1
printf '%s\n' '{"schemaVersion":1,"success":true,"result":{"passwordless":false,"version":"` + version + `","protocolVersion":3,"vault":true,"unlocked":true,"sessions":0}}'
`
}

// writeFakeCommand は、PATHに置く偽物のコマンドを実行できる形で書く。
func writeFakeCommand(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

// runInstallScript は、github.comにログインしたghのあるマシンでinstall.shを実行する。
// assetBodyは配布物のsshcとして渡す実行可能なスクリプトである。
func runInstallScript(t *testing.T, assetBody string, extraEnvironment ...string) installScriptRun {
	t.Helper()
	return runInstallScriptWith(t, installScriptSetup{
		assetBody:   assetBody,
		gitHubCLI:   signedInGitHubCLI,
		environment: extraEnvironment,
	})
}

// runInstallScriptWith は、install.shのHTTP境界、GitHub CLI、マシンの判定だけを偽物にして
// 実行する。checksum・same-directory staging・receipt publicationは実際のshellで通す。
func runInstallScriptWith(t *testing.T, setup installScriptSetup) installScriptRun {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("install.sh only supports Linux and macOS")
	}
	root := t.TempDir()
	fixtures := filepath.Join(root, "fixtures")
	commands := filepath.Join(root, "commands")
	systemTools := filepath.Join(root, "system")
	installDirectory := filepath.Join(root, "installed")
	for _, directory := range []string{fixtures, commands, systemTools, installDirectory} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	linkInstallScriptSystemTools(t, systemTools)
	assetBody := setup.assetBody

	digest := sha256.Sum256([]byte(assetBody))
	var checksums strings.Builder
	for _, assetName := range installScriptAssets {
		if err := os.WriteFile(filepath.Join(fixtures, assetName), []byte(assetBody), 0o755); err != nil {
			t.Fatal(err)
		}
		checksums.WriteString(hex.EncodeToString(digest[:]) + "  " + assetName + "\n")
	}
	if err := os.WriteFile(filepath.Join(fixtures, "checksums.txt"), []byte(checksums.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, body := range installScriptCommands {
		writeFakeCommand(t, filepath.Join(commands, name), body)
	}
	if setup.gitHubCLI != "" {
		writeFakeCommand(t, filepath.Join(commands, "gh"), setup.gitHubCLI)
	}
	if setup.runningEngineVersion != "" {
		writeFakeCommand(t, filepath.Join(commands, "sshc"), runningEngineCommand(setup.runningEngineVersion))
	}
	gitHubCLICalls := filepath.Join(root, "gh-calls")

	script, err := filepath.Abs(filepath.Join("..", "..", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("sh", script)
	environment := make([]string, 0, len(os.Environ())+8+len(setup.environment))
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "SHELL=") {
			continue
		}
		environment = append(environment, entry)
	}
	environment = append(environment,
		"PATH="+commands+":"+systemTools,
		"HOME="+filepath.Join(root, "home"),
		"SSHC_VERSION=v9.8.7",
		"SSHC_INSTALL_DIR="+installDirectory,
		"SSHC_TEST_FIXTURES="+fixtures,
		"SSHC_TEST_UNAME_SYSTEM=Linux",
		"SSHC_TEST_UNAME_MACHINE=x86_64",
		"SSHC_TEST_GH_CALLS="+gitHubCLICalls,
	)
	// 同じ名前が重なると後の値が使われるので、呼び出し側の指定が既定を上書きする。
	command.Env = append(environment, setup.environment...)
	output, err := command.CombinedOutput()
	calls, readErr := os.ReadFile(gitHubCLICalls)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatal(readErr)
	}
	return installScriptRun{
		output:           string(output),
		err:              err,
		installDirectory: installDirectory,
		assetDigest:      hex.EncodeToString(digest[:]),
		gitHubCLICalls:   string(calls),
	}
}

// installReceipt は、install.shが置き先の隣に書くreceiptである。
type installReceipt struct {
	SchemaVersion int    `json:"schemaVersion"`
	Manager       string `json:"manager"`
	Repository    string `json:"repository"`
	Version       string `json:"version"`
	SHA256        string `json:"sha256"`
}

// readInstallReceipt は、installDirectoryのreceiptを読む。無ければfalseを返す。
func readInstallReceipt(t *testing.T, installDirectory string) (installReceipt, bool) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(installDirectory, ".sshc-install-receipt.json"))
	if os.IsNotExist(err) {
		return installReceipt{}, false
	}
	if err != nil {
		t.Fatal(err)
	}
	var receipt installReceipt
	if err := json.Unmarshal(body, &receipt); err != nil {
		t.Fatal(err)
	}
	return receipt, true
}

// fixtureVersionLine は、platform（"linux/amd64"の形）向けの配布物のsshcが`sshc version`で出す行である。
func fixtureVersionLine(platform string) string {
	return "sshc v9.8.7 " + platform
}

// assetReporting は、`sshc version`でversionLineを出す配布物のsshcの本文である。
func assetReporting(versionLine string) string {
	return "#!/bin/sh\nprintf '%s\\n' '" + versionLine + "'\n"
}

// assertNoStagingFilesRemain は、途中で作った一時ファイルが置き先に残っていないことを確かめる。
func assertNoStagingFilesRemain(t *testing.T, installDirectory string) {
	t.Helper()
	entries, err := os.ReadDir(installDirectory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".sshc.install.") || strings.HasPrefix(entry.Name(), ".sshc.receipt.") {
			t.Errorf("staging file remains: %s", entry.Name())
		}
	}
}

func TestInstallScriptPublishesAReceiptForTheExactBinary(t *testing.T) {
	assetBody := assetReporting(fixtureVersionLine(defaultFixturePlatform))
	run := runInstallScript(t, assetBody)
	if run.err != nil {
		t.Fatalf("install.sh = %v\n%s", run.err, run.output)
	}
	if want := "sshc: installed " + fixtureVersionLine(defaultFixturePlatform) + "\n"; !strings.Contains(run.output, want) {
		t.Fatalf("install.sh did not report the installed version %q:\n%s", want, run.output)
	}
	target := filepath.Join(run.installDirectory, "sshc")
	installed, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != assetBody {
		t.Fatal("installed binary is not the verified fixture")
	}

	receipt, ok := readInstallReceipt(t, run.installDirectory)
	if !ok {
		t.Fatal("install.sh wrote no receipt for a stable release")
	}
	if receipt.SchemaVersion != 1 || receipt.Manager != "install.sh" || receipt.Repository != "aida0710/sshc" ||
		receipt.Version != "v9.8.7" || receipt.SHA256 != run.assetDigest {
		t.Fatalf("receipt = %#v", receipt)
	}
	assertNoStagingFilesRemain(t, run.installDirectory)
}

// sshc updateとsshc service installは安定バージョンのreceiptだけを受け付ける。prereleaseを
// 入れたときにreceiptを書くと、読み手はそれを壊れたreceiptとして扱い、原因と違うエラーで止まる。
// 前の安定版のreceiptが残っていても、別のbinaryを指すreceiptとして同じく止まる。
func TestInstallScriptLeavesAPrereleaseWithoutAReceipt(t *testing.T) {
	stable := runInstallScript(t, assetReporting(fixtureVersionLine(defaultFixturePlatform)))
	if stable.err != nil {
		t.Fatalf("install.sh = %v\n%s", stable.err, stable.output)
	}
	receipt := filepath.Join(stable.installDirectory, ".sshc-install-receipt.json")
	if _, err := os.Stat(receipt); err != nil {
		t.Fatalf("the stable install wrote no receipt: %v", err)
	}

	prereleaseBody := assetReporting("sshc v9.8.8-rc.1 " + defaultFixturePlatform)
	// 同じ置き先へ入れ直し、前の安定版のreceiptが消えることも確かめる。
	prerelease := runInstallScript(t, prereleaseBody,
		"SSHC_VERSION=v9.8.8-rc.1", "SSHC_INSTALL_DIR="+stable.installDirectory)
	if prerelease.err != nil {
		t.Fatalf("install.sh = %v\n%s", prerelease.err, prerelease.output)
	}
	installed, err := os.ReadFile(filepath.Join(stable.installDirectory, "sshc"))
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != prereleaseBody {
		t.Fatal("installed binary is not the verified prerelease fixture")
	}
	if _, err := os.Lstat(receipt); !os.IsNotExist(err) {
		t.Errorf("install.sh left a receipt for the prerelease: %v", err)
	}
	if !strings.Contains(prerelease.output, "sshc update and sshc service install do not manage this copy") {
		t.Errorf("install.sh did not say that sshc update and sshc service install skip the prerelease:\n%s", prerelease.output)
	}
	assertNoStagingFilesRemain(t, stable.installDirectory)
}

// /tmpをnoexecでマウントしたマシンでも入れられるよう、取得物はTMPDIRではなく置き先で実行する。
func TestInstallScriptRunsTheDownloadOnlyFromTheInstallDirectory(t *testing.T) {
	executions := filepath.Join(t.TempDir(), "executions")
	assetBody := "#!/bin/sh\nprintf '%s\\n' \"$0\" >> \"$SSHC_TEST_EXECUTIONS\"\nprintf '%s\\n' '" +
		fixtureVersionLine(defaultFixturePlatform) + "'\n"
	run := runInstallScript(t, assetBody, "SSHC_TEST_EXECUTIONS="+executions)
	if run.err != nil {
		t.Fatalf("install.sh = %v\n%s", run.err, run.output)
	}
	recorded, err := os.ReadFile(executions)
	if err != nil {
		t.Fatal(err)
	}
	// 1行に1つのパスを記録している。TMPDIRに空白を含むマシンでもパスを途中で切らないよう、
	// 空白ではなく改行で分ける。
	for line := range strings.Lines(string(recorded)) {
		executed := strings.TrimSuffix(line, "\n")
		if filepath.Dir(executed) != run.installDirectory {
			t.Errorf("install.sh ran the download as %s, outside the install directory %s", executed, run.installDirectory)
		}
	}
}

// 取得物を実行できないときは、シェルが出した理由を隠さずに表示し、何も置かない。
func TestInstallScriptShowsWhyTheDownloadCouldNotRun(t *testing.T) {
	assetBody := "#!/bin/sh\necho 'simulated: Permission denied' >&2\nexit 126\n"
	run := runInstallScript(t, assetBody)
	if run.err == nil {
		t.Fatalf("install.sh succeeded with a download that cannot run:\n%s", run.output)
	}
	if !strings.Contains(run.output, "does not report its version: simulated: Permission denied") {
		t.Errorf("install.sh hid why the download could not run:\n%s", run.output)
	}
	if _, err := os.Lstat(filepath.Join(run.installDirectory, "sshc")); !os.IsNotExist(err) {
		t.Errorf("install.sh placed sshc although it could not run: %v", err)
	}
	assertNoStagingFilesRemain(t, run.installDirectory)
}

// assertNothingWasInstalled は、install.shが止まったときに置き先へ実行ファイルもreceiptも
// 一時ファイルも残していないことを確かめる。
func assertNothingWasInstalled(t *testing.T, installDirectory string) {
	t.Helper()
	for _, name := range []string{"sshc", ".sshc-install-receipt.json"} {
		if _, err := os.Lstat(filepath.Join(installDirectory, name)); !os.IsNotExist(err) {
			t.Errorf("install.sh left %s although it stopped: %v", name, err)
		}
	}
	assertNoStagingFilesRemain(t, installDirectory)
}

// Rosetta 2の下で動くシェルでもuname -mはx86_64を返す。Apple SiliconのMacにはarm64版を入れ、
// IntelのMacにはamd64版を入れる。
func TestInstallScriptChoosesTheBuildForTheMacCPUEvenUnderRosetta(t *testing.T) {
	for _, test := range []struct {
		name string
		// arm64CPU は、偽のsysctlがhw.optional.arm64として返す値である。空ならそのOIDは無い。
		arm64CPU        string
		wantPlatform    string
		wantRosettaNote bool
	}{
		{name: "Apple Silicon under Rosetta 2", arm64CPU: "1", wantPlatform: "darwin/arm64", wantRosettaNote: true},
		{name: "Intel Mac", wantPlatform: "darwin/amd64"},
	} {
		t.Run(test.name, func(t *testing.T) {
			run := runInstallScript(t, assetReporting(fixtureVersionLine(test.wantPlatform)),
				"SSHC_TEST_UNAME_SYSTEM=Darwin", "SSHC_TEST_UNAME_MACHINE=x86_64", "SSHC_TEST_ARM64_CPU="+test.arm64CPU)
			if run.err != nil {
				t.Fatalf("install.sh = %v\n%s", run.err, run.output)
			}
			asset := "sshc-" + strings.Replace(test.wantPlatform, "/", "-", 1)
			if !strings.Contains(run.output, "sshc: installing "+asset+" (v9.8.7)") {
				t.Errorf("install.sh did not choose %s:\n%s", asset, run.output)
			}
			if got := strings.Contains(run.output, "Rosetta 2"); got != test.wantRosettaNote {
				t.Errorf("install.sh mentioned Rosetta 2 = %v, want %v:\n%s", got, test.wantRosettaNote, run.output)
			}
		})
	}
}

// 取得物が選んだ成果物と違うOS・CPU・バージョンを名乗るなら、それをreceiptに記録せず、何も置かない。
func TestInstallScriptRefusesADownloadThatNamesAnotherBuild(t *testing.T) {
	for _, test := range []struct {
		name        string
		versionLine string
		wantError   string
	}{
		{name: "another CPU", versionLine: "sshc v9.8.7 linux/arm64", wantError: "does not identify itself as sshc for linux/amd64"},
		{name: "another OS", versionLine: "sshc v9.8.7 darwin/amd64", wantError: "does not identify itself as sshc for linux/amd64"},
		{name: "another version", versionLine: "sshc v9.8.6 linux/amd64", wantError: "reports v9.8.6, expected v9.8.7"},
		{name: "not sshc", versionLine: "hello", wantError: "does not identify itself as sshc for linux/amd64: hello"},
	} {
		t.Run(test.name, func(t *testing.T) {
			run := runInstallScript(t, assetReporting(test.versionLine))
			if run.err == nil {
				t.Fatalf("install.sh installed a download that reports %q:\n%s", test.versionLine, run.output)
			}
			if !strings.Contains(run.output, test.wantError) {
				t.Errorf("install.sh did not say why it refused the download (want %q):\n%s", test.wantError, run.output)
			}
			assertNothingWasInstalled(t, run.installDirectory)
		})
	}
}

// 置き先に同じ名前のディレクトリがあれば、receiptを書き換える前、ダウンロードする前に止まる。
func TestInstallScriptRefusesADirectoryInPlaceOfTheExecutable(t *testing.T) {
	installDirectory := t.TempDir()
	occupied := filepath.Join(installDirectory, "sshc")
	if err := os.Mkdir(occupied, 0o755); err != nil {
		t.Fatal(err)
	}
	run := runInstallScript(t, assetReporting(fixtureVersionLine(defaultFixturePlatform)),
		"SSHC_INSTALL_DIR="+installDirectory)
	if run.err == nil {
		t.Fatalf("install.sh succeeded although %s is a directory:\n%s", occupied, run.output)
	}
	if !strings.Contains(run.output, occupied+" is a directory") {
		t.Errorf("install.sh did not say that the target is a directory:\n%s", run.output)
	}
	if info, err := os.Lstat(occupied); err != nil || !info.IsDir() {
		t.Errorf("install.sh changed the directory at the target: %v", err)
	}
	entries, err := os.ReadDir(occupied)
	if err != nil || len(entries) != 0 {
		t.Errorf("install.sh wrote into the directory at the target: %v %v", entries, err)
	}
	if _, err := os.Lstat(filepath.Join(installDirectory, ".sshc-install-receipt.json")); !os.IsNotExist(err) {
		t.Errorf("install.sh wrote a receipt although it could not install: %v", err)
	}
	assertNoStagingFilesRemain(t, installDirectory)
}

// 実行ファイルとreceiptのrenameのどちらで止まっても、残ったreceiptが別の実行ファイルを指さない。
// 指していると、sshc updateとsshc service installがdigestの不一致で止まり続ける。
func TestInstallScriptNeverLeavesAReceiptForAnotherExecutable(t *testing.T) {
	for _, failedRename := range []string{"sshc", ".sshc-install-receipt.json"} {
		t.Run(failedRename, func(t *testing.T) {
			previous := runInstallScript(t, assetReporting(fixtureVersionLine(defaultFixturePlatform)))
			if previous.err != nil {
				t.Fatalf("install.sh = %v\n%s", previous.err, previous.output)
			}
			installDirectory := previous.installDirectory
			updated := runInstallScript(t, assetReporting("sshc v9.8.8 "+defaultFixturePlatform),
				"SSHC_VERSION=v9.8.8", "SSHC_INSTALL_DIR="+installDirectory,
				"SSHC_TEST_FAIL_MV_TO="+filepath.Join(installDirectory, failedRename))
			if updated.err == nil {
				t.Fatalf("install.sh succeeded although renaming %s failed:\n%s", failedRename, updated.output)
			}
			if !strings.Contains(updated.output, "run this script again") {
				t.Errorf("install.sh did not say how to recover:\n%s", updated.output)
			}
			if receipt, ok := readInstallReceipt(t, installDirectory); ok {
				executable, err := os.ReadFile(filepath.Join(installDirectory, "sshc"))
				if err != nil {
					t.Fatal(err)
				}
				digest := sha256.Sum256(executable)
				if receipt.SHA256 != hex.EncodeToString(digest[:]) {
					t.Errorf("the receipt describes %s %s, not the executable left at the target", receipt.Version, receipt.SHA256)
				}
			}
			assertNoStagingFilesRemain(t, installDirectory)
		})
	}
}

// ghがgithub.comにログインしていれば、checksumに加えて、このリポジトリのRelease workflowが
// GitHub-hosted runnerで署名したattestationを確かめる。checksums.txtは成果物と同じReleaseから
// 取るので、Releaseを書き換えられるとchecksumだけでは気付けない。
func TestInstallScriptChecksTheReleaseAttestationWhenGitHubCLIIsSignedIn(t *testing.T) {
	const pinnedTag = "v9.8.7"
	for _, test := range []struct {
		name string
		// version は、SSHC_VERSIONに渡す値である。空なら最新版を入れる。
		version    string
		wantSigner string
	}{
		{name: "pinned tag", version: pinnedTag, wantSigner: publishedReleaseSigner(t, pinnedTag)},
		{
			name:       "latest release",
			wantSigner: `^https://github\.com/aida0710/sshc/\.github/workflows/release\.yml@refs/(tags/v[^/]+|heads/main)$`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			run := runInstallScript(t, assetReporting(fixtureVersionLine(defaultFixturePlatform)),
				"SSHC_VERSION="+test.version)
			if run.err != nil {
				t.Fatalf("install.sh = %v\n%s", run.err, run.output)
			}
			wantArguments := " --repo aida0710/sshc --cert-identity-regex " + test.wantSigner + " --deny-self-hosted-runners"
			verifications := attestationVerifications(run.gitHubCLICalls)
			if len(verifications) != 1 || !strings.HasSuffix(verifications[0], wantArguments) {
				t.Errorf("install.sh did not verify the attestation once with%s\ngh calls:\n%s", wantArguments, run.gitHubCLICalls)
			}
			if !strings.Contains(run.output, "attestation matches the Release workflow of aida0710/sshc") {
				t.Errorf("install.sh did not report the attestation check:\n%s", run.output)
			}
		})
	}
}

// publishedReleaseSigner は、scripts/release/publish.shが公開後の検証でtagの成果物に求める
// 署名者（証明書のSAN）の正規表現である。install.shもタグを固定したときは同じ署名者だけを
// 受け付ける。
func publishedReleaseSigner(t *testing.T, tag string) string {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is required to read publish.sh")
	}
	publish, err := filepath.Abs(filepath.Join("..", "..", "scripts", "release", "publish.sh"))
	if err != nil {
		t.Fatal(err)
	}
	script := `eval "$(sed -n '/^regex_literal()/,/^}/p; /^release_signer_pattern()/,/^}/p' "$1")"
repository=aida0710/sshc
tag=$2
release_signer_pattern`
	output, err := exec.Command("bash", "-c", script, "bash", publish, tag).Output()
	if err != nil || len(output) == 0 {
		t.Fatalf("read the signer from publish.sh: %v %q", err, output)
	}
	return string(output)
}

// 検証が走って通らなかった成果物は、checksumが合っていても置かない。署名の不一致とネットワークや
// APIの失敗を読み分けられるよう、理由を決めつけずghのエラーをそのまま見せる。
func TestInstallScriptRefusesADownloadWhoseAttestationDidNotVerify(t *testing.T) {
	for _, test := range []struct {
		name string
		// attestationError は、偽のghがattestation verifyの失敗として表示するエラーである。
		attestationError string
	}{
		{name: "another signer", attestationError: "simulated: no attestation matched the signer"},
		{name: "API failure", attestationError: "simulated: failed to fetch attestations: HTTP 502"},
	} {
		t.Run(test.name, func(t *testing.T) {
			run := runInstallScript(t, assetReporting(fixtureVersionLine(defaultFixturePlatform)),
				"SSHC_TEST_ATTESTATION_ERROR="+test.attestationError)
			if run.err == nil {
				t.Fatalf("install.sh installed a download whose attestation did not verify:\n%s", run.output)
			}
			for _, want := range []string{
				"gh attestation verify did not confirm that the Release workflow of aida0710/sshc signed the download, so nothing was installed",
				test.attestationError,
			} {
				if !strings.Contains(run.output, want) {
					t.Errorf("install.sh did not show %q:\n%s", want, run.output)
				}
			}
			assertNothingWasInstalled(t, run.installDirectory)
		})
	}
}

// ghが無い、attestationを検証できない古いgh、またはログインしていないマシンでも入れられる。
// そのときは、attestationを確かめずchecksumだけで入れたことを表示する。検証できないghを
// 署名の不一致と取り違えて止めると、同じマシンではsshc updateも毎回止まる。
func TestInstallScriptSaysWhenItCheckedOnlyTheChecksum(t *testing.T) {
	for _, test := range []struct {
		name      string
		gitHubCLI string
		wantNote  string
	}{
		{name: "no GitHub CLI", wantNote: "GitHub CLI (gh) is not installed"},
		{
			name:      "GitHub CLI older than gh attestation",
			gitHubCLI: gitHubCLIWithoutAttestation,
			wantNote:  "GitHub CLI (gh) cannot verify attestations (gh 2.49.0 or later is needed)",
		},
		{name: "signed out", gitHubCLI: signedOutGitHubCLI, wantNote: "GitHub CLI (gh) is not signed in to github.com"},
	} {
		t.Run(test.name, func(t *testing.T) {
			run := runInstallScriptWith(t, installScriptSetup{
				assetBody: assetReporting(fixtureVersionLine(defaultFixturePlatform)),
				gitHubCLI: test.gitHubCLI,
			})
			if run.err != nil {
				t.Fatalf("install.sh = %v\n%s", run.err, run.output)
			}
			if !strings.Contains(run.output, test.wantNote+", so the release attestation was not checked; only the checksum was") {
				t.Errorf("install.sh did not say that it checked only the checksum:\n%s", run.output)
			}
			if verifications := attestationVerifications(run.gitHubCLICalls); len(verifications) != 0 {
				t.Errorf("install.sh asked gh to verify the attestation although it could not:\n%s", run.gitHubCLICalls)
			}
		})
	}
}

// 動いているengineとバージョンが違うときは、置き換える前に、常駐サービスとほかのengineの
// 再起動の仕方を案内する。配布していないデスクトップアプリは案内しない。
func TestInstallScriptTellsHowToRestartAnEngineOfAnotherVersion(t *testing.T) {
	run := runInstallScriptWith(t, installScriptSetup{
		assetBody:            assetReporting(fixtureVersionLine(defaultFixturePlatform)),
		gitHubCLI:            signedInGitHubCLI,
		runningEngineVersion: "v9.8.6",
	})
	if run.err != nil {
		t.Fatalf("install.sh = %v\n%s", run.err, run.output)
	}
	for _, want := range []string{
		"an engine is running version v9.8.6; after this it will not match v9.8.7",
		"restart the managed service with `sshc service install`, or any other engine with `sshc engine --replace`",
	} {
		if !strings.Contains(run.output, want) {
			t.Errorf("install.sh did not say %q:\n%s", want, run.output)
		}
	}
	if strings.Contains(run.output, "sshc app") {
		t.Errorf("install.sh points to a desktop app that is not distributed:\n%s", run.output)
	}
}

// sshc updateから呼ばれたときは、再起動の案内をsshc updateに任せ、同じ案内を二度出さない。
func TestInstallScriptLeavesTheRestartAdviceToSshcUpdate(t *testing.T) {
	run := runInstallScriptWith(t, installScriptSetup{
		assetBody:            assetReporting(fixtureVersionLine(defaultFixturePlatform)),
		gitHubCLI:            signedInGitHubCLI,
		environment:          []string{"SSHC_INSTALL_CALLER=update"},
		runningEngineVersion: "v9.8.6",
	})
	if run.err != nil {
		t.Fatalf("install.sh = %v\n%s", run.err, run.output)
	}
	if !strings.Contains(run.output, "an engine is running version v9.8.6") {
		t.Errorf("install.sh did not say that another version is running:\n%s", run.output)
	}
	if strings.Contains(run.output, "sshc engine --replace") {
		t.Errorf("install.sh repeated the restart advice that sshc update gives:\n%s", run.output)
	}
}
