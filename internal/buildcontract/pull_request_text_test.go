package buildcontract

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// PR のタイトル・本文とブランチのコミットメッセージは、scripts/ci/check-pr-text.sh が検査する。
// PR の CI（.github/workflows/pull-request-text.yml）と make check-commits が同じスクリプトを使う。

func pullRequestTextScript(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the script runs on the Linux runner and from make on Unix")
	}
	script, err := filepath.Abs(filepath.Join("..", "..", "scripts", "ci", "check-pr-text.sh"))
	if err != nil {
		t.Fatal(err)
	}
	return script
}

// isolatedGitEnvironment は、手元の git の設定（署名、既定のブランチ、利用者の名前）を読まない環境である。
// CI の runner には利用者の名前が無いので、手元でだけ通るテストにしないためである。
func isolatedGitEnvironment() []string {
	var environment []string
	for _, variable := range os.Environ() {
		name, _, _ := strings.Cut(variable, "=")
		if name == "PR_TITLE" || name == "PR_BODY" || strings.HasPrefix(name, "GIT_") {
			continue
		}
		environment = append(environment, variable)
	}
	return append(environment, "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
}

// pullRequestText は、check-pr-text.sh に渡すものである。
type pullRequestText struct {
	// env は、PR_TITLE・PR_BODY のように環境変数で渡す値である。
	env map[string]string
	// directory は、コミットを検査するリポジトリである。空なら今のディレクトリで実行する。
	directory string
	// revisions は、<base> <head> の2つか、空である。
	revisions []string
}

// checkPullRequestText は、check-pr-text.sh を実行し、違反が無かったかと標準エラーを返す。
// 使い方の誤り（終了の状態 2）は、テストの書き方の誤りなので、その場で止める。
func checkPullRequestText(t *testing.T, input pullRequestText) (bool, string) {
	t.Helper()
	command := exec.Command("bash", append([]string{pullRequestTextScript(t)}, input.revisions...)...)
	command.Dir = input.directory
	command.Env = isolatedGitEnvironment()
	for name, value := range input.env {
		command.Env = append(command.Env, name+"="+value)
	}
	var stderr strings.Builder
	command.Stderr = &stderr
	err := command.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return true, stderr.String()
	case errors.As(err, &exit) && exit.ExitCode() == 1:
		return false, stderr.String()
	}
	t.Fatalf("run check-pr-text.sh: %v\n%s", err, stderr.String())
	return false, ""
}

// タイトルは、squash でマージすると main のコミットの件名になる。
func TestPullRequestTitleNeedsATypeAndAJapaneseSummary(t *testing.T) {
	cases := []struct {
		title    string
		accepted bool
	}{
		{"fix(sftp): 一覧の並び順を直す", true},
		{"docs: 手順を足す", true},
		{"feat(web)!: 設定の形を変える", true},
		{"refactor(terminal,sftp): 名前をそろえる", true},
		{"ci: PR のタイトルを検査する", true},
		{"fix(sftp): sort the list", false},
		{"一覧の並び順を直す", false},
		{"update(sftp): 一覧の並び順を直す", false},
		{"Fix(sftp): 一覧の並び順を直す", false},
		{"fix(sftp):一覧の並び順を直す", false},
		{"fix(): 一覧の並び順を直す", false},
	}
	for _, testCase := range cases {
		t.Run(testCase.title, func(t *testing.T) {
			accepted, stderr := checkPullRequestText(t, pullRequestText{env: map[string]string{"PR_TITLE": testCase.title}})
			if accepted != testCase.accepted {
				t.Fatalf("title %q accepted = %v, want %v\n%s", testCase.title, accepted, testCase.accepted, stderr)
			}
		})
	}
}

// AI ツールが追加する帰属の行と、社内の URL は、公開リポジトリの本文に入れない。
func TestPullRequestBodyRejectsAttributionAndInternalURLs(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		accepted bool
	}{
		{"a plain body", "## 何が問題か\n一覧の並びが崩れる。", true},
		{"no body", "", true},
		{"an image on the issue-assets branch", `<img src="https://raw.githubusercontent.com/aida0710/sshc/issue-assets/sftp/list.png" width="720">`, true},
		{"loopback", "http://127.0.0.1:18930/ で開く", true},
		{"an address outside RFC 1918", "http://172.32.0.1/", true},
		{"internal in the path", "https://example.com/internal/page", true},
		{"internal in the middle of the host", "https://pelican.internal.example.com/", true},
		{"generated with in an English sentence", "the models are generated with oapi-codegen", true},
		{"a session line", "fix(sftp): 直す\n\nClaude-Session: https://claude.ai/code/session_0123", false},
		{"a co-author line in any case", "Co-authored-by: Dunlin <dunlin@example.invalid>", false},
		{"a generated-with line", "🤖 Generated with [Claude Code](https://claude.com/claude-code)", false},
		{"a bare session link", "https://claude.ai/code/session_0123", false},
		{"an internal host", "画像: http://share.example.internal/sshc/pack/", false},
		{"an internal host before Japanese punctuation", "画像（http://mirage.internal）", false},
		{"an internal host at the end of a sentence", "ここで見られる: https://mirage.internal.", false},
		{"a 10/8 address with a port", "http://10.1.2.3:8930/sshc/", false},
		{"a 192.168/16 address", "https://192.168.0.10/", false},
		{"a 172.16/12 address", "http://172.16.0.5/", false},
		{"an address after user information", "http://pelican@10.20.30.40/", false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			accepted, stderr := checkPullRequestText(t, pullRequestText{env: map[string]string{"PR_BODY": testCase.body}})
			if accepted != testCase.accepted {
				t.Fatalf("body %q accepted = %v, want %v\n%s", testCase.body, accepted, testCase.accepted, stderr)
			}
		})
	}
}

// 公開リポジトリの Actions のログは誰でも読めるので、消したい URL をログに書き写さない。
func TestPullRequestTextCheckNamesTheLineWithoutRepeatingTheURL(t *testing.T) {
	accepted, stderr := checkPullRequestText(t, pullRequestText{env: map[string]string{
		"PR_BODY": "## 確認したこと\n画像: http://share.example.internal/sshc/pack/",
	}})
	if accepted {
		t.Fatal("a body with an internal URL was accepted")
	}
	if strings.Contains(stderr, "example.internal") {
		t.Errorf("the report repeats the URL:\n%s", stderr)
	}
	if !strings.Contains(stderr, "2 行目") {
		t.Errorf("the report does not name line 2:\n%s", stderr)
	}
}

// squash の本文はブランチのコミットメッセージをつなげたものなので、コミットメッセージも main に入る。
// base から届くコミット（main に入っているもの）は、その PR の変更ではないので見ない。
func TestCommitMessagesAreCheckedOnlyBetweenBaseAndHead(t *testing.T) {
	repository := t.TempDir()
	runGit(t, repository, "init", "--quiet", "--initial-branch=main")
	base := commitWithMessage(t, repository, "chore: 最初のコミット\n\nCo-Authored-By: Dunlin <dunlin@example.invalid>")
	clean := commitWithMessage(t, repository, "fix(sftp): 一覧の並び順を直す")
	attributed := commitWithMessage(t, repository, "test(sftp): 並び順のテストを足す\n\nClaude-Session: https://claude.ai/code/session_0123")

	accepted, stderr := checkPullRequestText(t, pullRequestText{directory: repository, revisions: []string{base, clean}})
	if !accepted {
		t.Errorf("a clean branch on a base with an old attribution line was rejected:\n%s", stderr)
	}

	accepted, stderr = checkPullRequestText(t, pullRequestText{directory: repository, revisions: []string{base, attributed}})
	if accepted {
		t.Fatal("a branch with an attribution line in a commit message was accepted")
	}
	if !strings.Contains(stderr, shortCommit(t, repository, attributed)) {
		t.Errorf("the report does not name the commit with the attribution line:\n%s", stderr)
	}
	if strings.Contains(stderr, shortCommit(t, repository, clean)) {
		t.Errorf("the report names the clean commit:\n%s", stderr)
	}
}

func runGit(t *testing.T, repository string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{
		"-c", "user.name=pelican", "-c", "user.email=pelican@example.invalid", "-c", "commit.gpgsign=false",
	}, args...)...)
	command.Dir = repository
	command.Env = isolatedGitEnvironment()
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func commitWithMessage(t *testing.T, repository, message string) string {
	t.Helper()
	runGit(t, repository, "commit", "--quiet", "--allow-empty", "--message", message)
	return runGit(t, repository, "rev-parse", "HEAD")
}

func shortCommit(t *testing.T, repository, commit string) string {
	t.Helper()
	return runGit(t, repository, "rev-parse", "--short", commit)
}

// 本文だけを編集したとき（edited）は ci.yml が実行されないので、別の workflow で検査し直す。
func TestPullRequestTextWorkflowChecksAgainWhenTheTextIsEdited(t *testing.T) {
	document, source := readWorkflowFile(t, filepath.Join(workflowsDirectory(), "pull-request-text.yml"))

	var triggers struct {
		On struct {
			PullRequest struct {
				Types []string `yaml:"types"`
			} `yaml:"pull_request"`
		} `yaml:"on"`
	}
	if err := yaml.Unmarshal([]byte(source), &triggers); err != nil {
		t.Fatal(err)
	}
	for _, activity := range []string{"opened", "edited", "synchronize", "reopened"} {
		if !slices.Contains(triggers.On.PullRequest.Types, activity) {
			t.Errorf("pull-request-text.yml does not run when a pull request is %s", activity)
		}
	}

	for _, problem := range validateWorkflowPermissions(document, workflowPermissionContract{
		workflow: map[string]string{"contents": "read"},
	}) {
		t.Error(problem)
	}

	// 取り出すのは PR のマージコミットなので、1つ目の親が base、2つ目の親が PR の先頭になる。
	// その間のコミットを読むには、分かれた所まで履歴が要る。
	const checkCommand = "scripts/ci/check-pr-text.sh HEAD^1 HEAD^2"
	found := false
	for _, job := range document.Jobs {
		for _, step := range job.Steps {
			if strings.HasPrefix(step.Uses, "actions/checkout@") && step.With["fetch-depth"] != 0 {
				t.Errorf("the checkout fetches %v commits; the commits since the fork point need the whole history", step.With["fetch-depth"])
			}
			if strings.TrimSpace(step.Run) != checkCommand {
				continue
			}
			found = true
			if step.Env["PR_TITLE"] != "${{ github.event.pull_request.title }}" || step.Env["PR_BODY"] != "${{ github.event.pull_request.body }}" {
				t.Errorf("the check must receive the title and the body as PR_TITLE and PR_BODY, got %v", step.Env)
			}
		}
	}
	if !found {
		t.Errorf("pull-request-text.yml must run %q", checkCommand)
	}
}

// 手元の make check-commits と PR の CI は、同じスクリプトで検査する。
func TestMakeCheckCommitsRunsThePullRequestTextScript(t *testing.T) {
	recipe := requireTarget(t, readMakefileContract(t), "check-commits")
	if !strings.Contains(recipe, "scripts/ci/check-pr-text.sh") {
		t.Errorf("make check-commits does not run scripts/ci/check-pr-text.sh:\n%s", recipe)
	}
}
