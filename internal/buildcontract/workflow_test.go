package buildcontract

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type workflowDocument struct {
	Jobs map[string]workflowJob `yaml:"jobs"`
}

type workflowJob struct {
	Name     string            `yaml:"name"`
	RunsOn   string            `yaml:"runs-on"`
	Strategy *workflowStrategy `yaml:"strategy"`
	Steps    []workflowStep    `yaml:"steps"`
}

type workflowStrategy struct {
	FailFast *bool          `yaml:"fail-fast"`
	Matrix   workflowMatrix `yaml:"matrix"`
}

type workflowMatrix struct {
	Include []workflowMatrixEntry `yaml:"include"`
}

type workflowMatrixEntry struct {
	OS   string `yaml:"os"`
	Name string `yaml:"name"`
	Race *bool  `yaml:"race"`
}

type workflowStep struct {
	Name            string         `yaml:"name"`
	If              string         `yaml:"if"`
	Uses            string         `yaml:"uses"`
	Run             string         `yaml:"run"`
	Shell           string         `yaml:"shell"`
	With            map[string]any `yaml:"with"`
	ContinueOnError *bool          `yaml:"continue-on-error"`
}

func TestCIWorkflowProvidesNativeGoAndDesktopLifecycleMatrices(t *testing.T) {
	document := readWorkflowDocument(t)
	if problems := validateNativeWorkflow(document); len(problems) != 0 {
		t.Fatalf("native workflow contract violations:\n- %s", strings.Join(problems, "\n- "))
	}
}

func TestCIWorkflowKeepsWindowsRaceExceptionExact(t *testing.T) {
	path := workflowPath()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	tests := []struct {
		name string
		old  string
		new  string
	}{
		{
			name: "substring matching could hide a test or compile failure",
			old:  "$raceOutput -ceq '-race is not supported on windows/amd64'",
			new:  "$raceOutput -match 'race'",
		},
		{
			name: "success after every failure could hide a test or compile failure",
			old:  "exit $raceExit",
			new:  "exit 0",
		},
		{
			name: "a partial diagnostic is not the toolchain diagnostic",
			old:  "'-race is not supported on windows/amd64'",
			new:  "'not supported'",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !strings.Contains(string(source), test.old) {
				t.Fatalf("workflow does not contain the strict race-policy fragment %q", test.old)
			}
			mutated := strings.Replace(string(source), test.old, test.new, 1)
			document, err := decodeWorkflowDocument([]byte(mutated))
			if err != nil {
				t.Fatalf("decode mutated workflow: %v", err)
			}
			if problems := validateNativeWorkflow(document); len(problems) == 0 {
				t.Fatal("broadened Windows race exception unexpectedly satisfies the workflow contract")
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
		// Win32 rejects control characters 1 through 31 in file names, so the
		// native Windows fixture covers every representable adversarial name and
		// Unix covers the newline record that proves NUL, rather than lines, is
		// the transport boundary.
		tracked = append(tracked, "line\nbreak.go")
	}
	for _, name := range tracked {
		writeFixture(t, fixture, name, "package fixture\nfunc Unformatted( ) {}\n")
	}
	writeFixture(t, fixture, "ignored 日本語.go", "package fixture\nfunc Ignored( ) {}\n")
	addArgs := append([]string{"add", "--", "formatted.go"}, tracked...)
	runCommand(t, fixture, "git", addArgs...)

	rawPaths := runCommandOutput(t, fixture, "git", "ls-files", "-z", "--", "*.go")
	// Git sorts the index bytewise; formatted.go precedes line/space. Keep the
	// hand-derived literal explicit so line-oriented or quoted output fails.
	var wantRawPaths string
	if runtime.GOOS != "windows" {
		wantRawPaths = "-dash.go\x00formatted.go\x00line\nbreak.go\x00space name.go\x00日本語.go\x00"
	} else {
		wantRawPaths = "-dash.go\x00formatted.go\x00space name.go\x00日本語.go\x00"
	}
	if string(rawPaths) != wantRawPaths {
		t.Fatalf("git NUL path fixture = %q, want %q", rawPaths, wantRawPaths)
	}

	var command string
	var args []string
	if runtime.GOOS == "windows" {
		command = "pwsh"
		args = []string{"-NoProfile", "-File", filepath.Join(repository, "scripts", "ci", "check-gofmt.ps1")}
	} else {
		command = "sh"
		args = []string{filepath.Join(repository, "scripts", "ci", "check-gofmt.sh")}
	}

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
		"gofmt -l -- @goFiles",
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
	path := workflowPath()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	document, err := decodeWorkflowDocument(source)
	if err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return document
}

func decodeWorkflowDocument(source []byte) (workflowDocument, error) {
	var document workflowDocument
	if err := yaml.Unmarshal(source, &document); err != nil {
		return workflowDocument{}, err
	}
	if document.Jobs == nil {
		return workflowDocument{}, fmt.Errorf("jobs mapping is missing")
	}
	return document, nil
}

func workflowPath() string {
	return filepath.Join("..", "..", ".github", "workflows", "ci.yml")
}

func validateNativeWorkflow(document workflowDocument) []string {
	var problems []string
	goJob, ok := document.Jobs["go"]
	if !ok {
		problems = append(problems, "jobs.go is missing")
	} else {
		problems = append(problems, validateNativeMatrix("jobs.go", goJob, true)...)
		problems = append(problems, validateGoSteps(goJob)...)
		problems = append(problems, validatePinnedSetup(goJob, "actions/checkout")...)
		problems = append(problems, validatePinnedSetup(goJob, "actions/setup-go")...)
		problems = append(problems, validateSetupOrder("jobs.go", goJob, "actions/setup-go")...)
	}

	desktopJob, ok := document.Jobs["desktop"]
	if !ok {
		problems = append(problems, "jobs.desktop is missing")
	} else {
		problems = append(problems, validateNativeMatrix("jobs.desktop", desktopJob, false)...)
		problems = append(problems, validateDesktopSteps(desktopJob)...)
		problems = append(problems, validatePinnedSetup(desktopJob, "actions/checkout")...)
		problems = append(problems, validatePinnedSetup(desktopJob, "actions/setup-node")...)
		problems = append(problems, validateSetupOrder("jobs.desktop", desktopJob, "actions/setup-node")...)
	}

	if _, ok := document.Jobs["macos"]; ok {
		problems = append(problems, "the old jobs.macos duplicate must be folded into jobs.go")
	}
	for _, id := range []string{"web", "generated", "integration", "e2e", "security"} {
		job, ok := document.Jobs[id]
		if !ok {
			problems = append(problems, "single-instance job "+id+" is missing")
			continue
		}
		if job.Strategy != nil && len(job.Strategy.Matrix.Include) != 0 {
			problems = append(problems, "jobs."+id+" must remain single-instance")
		}
	}
	problems = append(problems, validateActionPins(document)...)
	return problems
}

func validateNativeMatrix(id string, job workflowJob, requireRace bool) []string {
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
		if requireRace && (entry.Race == nil || !*entry.Race) {
			problems = append(problems, id+" matrix entry "+entry.OS+" must set race: true")
		}
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

func validateGoSteps(job workflowJob) []string {
	var problems []string
	problems = append(problems, validateSeparatedRunShells("jobs.go", job)...)
	for _, required := range []runContract{
		{run: "scripts/ci/check-gofmt.sh", condition: "${{ runner.os != 'Windows' }}", shell: "bash"},
		{run: "./scripts/ci/check-gofmt.ps1", condition: "${{ runner.os == 'Windows' }}", shell: "pwsh"},
		{run: "go vet ./...", condition: "${{ runner.os != 'Windows' }}", shell: "bash"},
		{run: "go vet ./...", condition: "${{ runner.os == 'Windows' }}", shell: "pwsh"},
		{run: "go build ./...", condition: "${{ runner.os != 'Windows' }}", shell: "bash"},
		{run: "go build ./...", condition: "${{ runner.os == 'Windows' }}", shell: "pwsh"},
		{run: "go test ./...", condition: "${{ runner.os != 'Windows' }}", shell: "bash"},
		{run: "go test ./...", condition: "${{ runner.os == 'Windows' }}", shell: "pwsh"},
		{run: "go test -race ./...", condition: "${{ runner.os != 'Windows' }}", shell: "bash"},
	} {
		if !hasRunContract(job, required) {
			problems = append(problems, fmt.Sprintf("jobs.go lacks run=%q if=%q shell=%q", required.run, required.condition, required.shell))
		}
	}

	windowsRace, ok := namedStep(job, "go test -race (Windows)")
	if !ok {
		return append(problems, "jobs.go lacks the Windows race step")
	}
	if windowsRace.If != "${{ runner.os == 'Windows' }}" || windowsRace.Shell != "pwsh" {
		problems = append(problems, "the Windows race step must be Windows-only PowerShell")
	}
	if windowsRace.ContinueOnError != nil {
		problems = append(problems, "the Windows race step must not use continue-on-error")
	}
	for _, fragment := range []string{
		"$PSNativeCommandUseErrorActionPreference = $false",
		"@(& go test -race ./... 2>&1)",
		"$raceExit = $LASTEXITCODE",
		"if ($raceExit -eq 0)",
		"$raceOutput -ceq '-race is not supported on windows/amd64'",
		"Race unavailable on this runner/toolchain",
		"exit $raceExit",
	} {
		if !strings.Contains(windowsRace.Run, fragment) {
			problems = append(problems, fmt.Sprintf("the Windows race step lacks strict fragment %q", fragment))
		}
	}
	for _, forbidden := range []string{"-match", "-like", ".Contains("} {
		if strings.Contains(windowsRace.Run, forbidden) {
			problems = append(problems, fmt.Sprintf("the Windows race step uses broad matching %q", forbidden))
		}
	}
	return problems
}

func validateDesktopSteps(job workflowJob) []string {
	var problems []string
	problems = append(problems, validateSeparatedRunShells("jobs.desktop", job)...)
	if !strings.Contains(strings.ToLower(job.Name), "lifecycle") || strings.Contains(strings.ToLower(job.Name), "smoke") {
		problems = append(problems, "jobs.desktop name must identify lifecycle tests, not package/window smoke")
	}
	for _, command := range []string{"npm ci --prefix desktop", "npm test --prefix desktop"} {
		for _, platform := range []runContract{
			{run: command, condition: "${{ runner.os != 'Windows' }}", shell: "bash"},
			{run: command, condition: "${{ runner.os == 'Windows' }}", shell: "pwsh"},
		} {
			if !hasRunContract(job, platform) {
				problems = append(problems, fmt.Sprintf("jobs.desktop lacks run=%q if=%q shell=%q", platform.run, platform.condition, platform.shell))
			}
		}
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
		if fmt.Sprint(step.With["go-version-file"]) != "go.mod" || fmt.Sprint(step.With["cache-dependency-path"]) != "go.sum" {
			problems = append(problems, "native Go setup must use go.mod and go.sum")
		}
	}
	if action == "actions/setup-node" {
		if fmt.Sprint(step.With["node-version"]) != "22" || fmt.Sprint(step.With["cache"]) != "npm" || fmt.Sprint(step.With["cache-dependency-path"]) != "desktop/package-lock.json" {
			problems = append(problems, "desktop Node setup must use Node 22 and the desktop npm lockfile cache")
		}
	}
	return problems
}

func validateSeparatedRunShells(id string, job workflowJob) []string {
	var problems []string
	for _, step := range job.Steps {
		if step.Run == "" {
			continue
		}
		switch step.If {
		case "${{ runner.os != 'Windows' }}":
			if step.Shell != "bash" {
				problems = append(problems, fmt.Sprintf("%s step %q must use bash for its Unix-only command", id, step.Name))
			}
		case "${{ runner.os == 'Windows' }}":
			if step.Shell != "pwsh" {
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

func validateActionPins(document workflowDocument) []string {
	pinned := regexp.MustCompile(`^[^@]+@[0-9a-f]{40}$`)
	var problems []string
	ids := make([]string, 0, len(document.Jobs))
	for id := range document.Jobs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		for stepIndex, step := range document.Jobs[id].Steps {
			if step.Uses == "" || strings.HasPrefix(step.Uses, "./") {
				continue
			}
			if !pinned.MatchString(step.Uses) {
				problems = append(problems, fmt.Sprintf("jobs.%s step %d action is not pinned to a 40-hex commit: %q", id, stepIndex, step.Uses))
			}
		}
	}
	return problems
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
