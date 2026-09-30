package platform_test

import (
	"strings"
	"testing"

	"sshc/internal/platform"
)

func TestSanitiseHomePathsReplacesEveryOccurrence(t *testing.T) {
	const home = "/Users/tester"
	// 本物の `ssh -v` の出力は、読み込んだ設定ファイルと identity ファイルを、
	// それぞれ絶対パスで指定する。
	verbose := "debug1: Reading configuration data /Users/tester/.ssh/config\r\n" +
		"debug1: identity file /Users/tester/.ssh/id_ed25519 type 3\n" +
		"debug1: Authenticated to bastion ([203.0.113.10]:22).\n"

	sanitised := platform.SanitiseHomePaths(verbose, home)
	if strings.Contains(sanitised, home) {
		t.Fatalf("sanitised output still names the home directory: %q", sanitised)
	}
	if !strings.Contains(sanitised, "~/.ssh/config") || !strings.Contains(sanitised, "~/.ssh/id_ed25519") {
		t.Fatalf("sanitised = %q, want the paths rewritten to ~", sanitised)
	}
	if !strings.Contains(sanitised, "Authenticated to bastion") {
		t.Error("sanitising removed information the user needs")
	}
}

func TestSanitiseHomePathsLeavesTextAloneWhenThereIsNoHome(t *testing.T) {
	const text = "debug1: Connecting to 203.0.113.10 port 22.\n"

	if got := platform.SanitiseHomePaths(text, ""); got != text {
		t.Errorf("SanitiseHomePaths with no home = %q, want the text unchanged", got)
	}
	if got := platform.SanitiseHomePaths(text, "/"); got != text {
		t.Errorf("SanitiseHomePaths with a root home = %q, want the text unchanged", got)
	}
}

// ホームと先頭が同じだけの別のパスは書き換えない。/home/alice を ~ice にすると、
// 「ice というユーザーのホーム」と読める。
func TestSanitiseHomePathsLeavesPathsThatOnlyShareTheHomeSpellingAlone(t *testing.T) {
	const home = "/home/al"
	for _, text := range []string{
		"no identity in /home/alice/.ssh/id",
		"no identity in /mnt/snap/home/al/.ssh/id",
		"no identity in /home/al.bak/.ssh/id",
	} {
		if got := platform.SanitiseHomePaths(text, home); got != text {
			t.Errorf("SanitiseHomePaths(%q) = %q, want the text unchanged", text, got)
		}
	}
}

// 鍵を読めなかったときの理由は、括弧やエラー文の ": " でパスを囲む。
func TestSanitiseHomePathsRewritesTheHomeWhereverItStartsAPath(t *testing.T) {
	const home = "/home/al"
	for text, want := range map[string]string{
		"(/home/al/.ssh/id: open /home/al/.ssh/id: permission denied)": "(~/.ssh/id: open ~/.ssh/id: permission denied)",
		"HOME=/home/al":          "HOME=~",
		"/home/al":               "~",
		`"/home/al/.ssh/config"`: `"~/.ssh/config"`,
	} {
		if got := platform.SanitiseHomePaths(text, home); got != want {
			t.Errorf("SanitiseHomePaths(%q) = %q, want %q", text, got, want)
		}
	}
}
