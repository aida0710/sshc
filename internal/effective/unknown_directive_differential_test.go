package effective_test

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"sshc/internal/config"
	"sshc/internal/sshmatch"
)

// badConfigurationOption は、ssh が知らないキーワードの行ごとに出す1行である
// （readconf.c の parse_token）。
var badConfigurationOption = regexp.MustCompile(`(?m)^(.+): line (\d+): Bad configuration option: (\S+)\r?$`)

// TestUnknownDirectivesMatchInstalledOpenSSH は、unknown_directive の診断と、本物の ssh が
// 「Bad configuration option」と言って止まる行が一致することを確かめる。どちらも
// 「.ssh からの相対パス:行:小文字のキーワード」で比べる。
//
// フィクスチャの書き方には3つの決まりがある。
//   - IgnoreUnknown が効くかは接続先で変わる。sshc はどこかの接続先で止まる行を知らせるので、
//     alias には、条件付きの IgnoreUnknown が効かない側の名前を選ぶ。
//   - ssh は、知らない行のあるファイルを読み終えたところで止まり、取り込み元の残りを読まない。
//     知らない行は、Include より後ろの取り込み元には置かない。
//   - キーワードは、CI の OpenSSH（Ubuntu 24.04 の 9.6p1、macOS、Windows に付いてくるもの）も
//     知っている古いものだけを使う。表は 10.2p1 から作ったので、新しいキーワードは古い ssh が止まる。
//
// 同じ場合分けを、ssh の無い環境でも internal/config の TestResolveReportsTheLinesOpenSSHStopsAt が固定する。
func TestUnknownDirectivesMatchInstalledOpenSSH(t *testing.T) {
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		t.Skip("OpenSSH ssh is not installed; skipping the differential test")
	}

	tests := []struct {
		name     string
		contents string
		files    map[string]string
		alias    string
	}{
		{
			name:     "a misspelt keyword inside another host",
			contents: "Host typo-host\n\tHostNmae 192.0.2.77\n\tUser deploy\nHost other\n\tHostName 192.0.2.1\n",
			alias:    "other",
		},
		{
			name:     "deprecated, unsupported and ignored keywords",
			contents: "Protocol 2\nUseRoaming no\nCipher blowfish\nRSAAuthentication yes\nCompressionLevel 6\n",
			alias:    "a",
		},
		{
			name:     "keywords in any case and with an equals sign",
			contents: "HOSTNAME 192.0.2.1\nHostName=192.0.2.2\nUser = deploy\nHostNmae=192.0.2.3\n",
			alias:    "a",
		},
		{
			name:     "an ignore unknown before the line",
			contents: "IgnoreUnknown hostnmae\nHost a\n\tHOSTNMAE 192.0.2.1\n",
			alias:    "a",
		},
		{
			name:     "an ignore unknown written with an equals sign and patterns",
			contents: "IgnoreUnknown=Foo,Host*\nHost a\n\tHostNmae 192.0.2.1\n",
			alias:    "a",
		},
		{
			name:     "an ignore unknown after the line",
			contents: "HostNmae 192.0.2.1\nIgnoreUnknown HostNmae\n",
			alias:    "a",
		},
		{
			name:     "a second ignore unknown",
			contents: "IgnoreUnknown Foo\nIgnoreUnknown HostNmae\nHost a\n\tHostNmae 192.0.2.1\n",
			alias:    "a",
		},
		{
			name:     "a negated pattern in ignore unknown",
			contents: "IgnoreUnknown *,!hostnmae\nHost a\n\tHostNmae 192.0.2.1\n\tUsr x\n",
			alias:    "a",
		},
		{
			name:     "the first ignore unknown every destination reads",
			contents: "Host *\n\tIgnoreUnknown Foo*\nMatch all\n\tIgnoreUnknown Bar\nHost a\n\tFooBar yes\n\tBar yes\n",
			alias:    "a",
		},
		{
			name:     "an ignore unknown that only another destination reads",
			contents: "Host probe\n\tIgnoreUnknown HostNmae\nHost a\n\tHostNmae 192.0.2.1\n",
			alias:    "a",
		},
		{
			name:     "a conditional ignore unknown read before one every destination reads",
			contents: "Host probe\n\tIgnoreUnknown Foo\nHost *\n\tIgnoreUnknown HostNmae\nHost a\n\tHostNmae 192.0.2.1\n",
			alias:    "probe",
		},
		{
			name:     "a host that also excludes a name",
			contents: "Host * !probe\n\tIgnoreUnknown HostNmae\nHost a\n\tHostNmae 192.0.2.1\n",
			alias:    "probe",
		},
		{
			name:     "an unknown keyword inside a match that matches nothing",
			contents: "Match host nothing.invalid\n\tHostNmae 192.0.2.1\n",
			alias:    "a",
		},
		{
			name:     "an unknown keyword in an included file",
			contents: "Include conf.d/*.conf\nHost after\n\tHostName 192.0.2.9\n",
			files:    map[string]string{"conf.d/work.conf": "Host work\n\tHostNmae 192.0.2.1\n"},
			alias:    "after",
		},
		{
			name:     "an ignore unknown in an included file",
			contents: "Include conf.d/*.conf\nHost a\n\tHostNmae 192.0.2.1\n",
			files:    map[string]string{"conf.d/10.conf": "Host *\n\tIgnoreUnknown HostNmae\n"},
			alias:    "a",
		},
		{
			name:     "an ignore unknown in a file included from inside another host",
			contents: "Host other\n\tInclude conf.d/*.conf\nHost a\n\tHostNmae 192.0.2.1\n",
			files:    map[string]string{"conf.d/10.conf": "Host *\n\tIgnoreUnknown HostNmae\n"},
			alias:    "a",
		},
		{
			name:     "an unknown keyword in a file included from inside another host",
			contents: "Host other\n\tInclude conf.d/*.conf\n",
			files:    map[string]string{"conf.d/10.conf": "Usr deploy\n"},
			alias:    "a",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// macOS の一時フォルダはシンボリックリンクの先にある。ssh とリゾルバが別の表記で
			// 同じファイルを指すと、相対パスがそろわない。
			home, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			configPath := writeSSHConfigFixture(t, home, test.contents, test.files)
			root := filepath.Dir(configPath)

			got := unknownDirectivesOf(t, resolveFixture(t, home, configPath), root)
			want := linesSSHStopsAt(t, sshPath, home, configPath, test.alias)
			if !slices.Equal(got, want) {
				t.Errorf("unknown_directive = %q, ssh -G stops at %q", got, want)
			}
		})
	}
}

// unknownDirectivesOf は、sshc が知らせる知らない行を並べる。
func unknownDirectivesOf(t *testing.T, graph *config.Graph, root string) []string {
	t.Helper()
	found := []string{}
	for _, diagnostic := range graph.Diagnostics {
		if diagnostic.Code != config.DiagnosticUnknownDirective {
			continue
		}
		found = append(found, fixtureLine(t, root, diagnostic.Path, fmt.Sprint(diagnostic.Line), sshmatch.LowerASCII(diagnostic.Detail)))
	}
	slices.Sort(found)
	return found
}

// linesSSHStopsAt は、本物の ssh が知らないと言った行を並べる。ssh が設定を読めたら空である。
func linesSSHStopsAt(t *testing.T, sshPath, home, configPath, alias string) []string {
	t.Helper()
	_, err := sshG(sshPath, home, configPath, alias)
	if err == nil {
		return []string{}
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("ssh -G = %v", err)
	}
	reported := badConfigurationOption.FindAllSubmatch(exit.Stderr, -1)
	if len(reported) == 0 {
		t.Fatalf("ssh -G stopped for another reason: %v\n%s", err, exit.Stderr)
	}
	root := filepath.Dir(configPath)
	found := make([]string, 0, len(reported))
	for _, line := range reported {
		found = append(found, fixtureLine(t, root, string(line[1]), string(line[2]), string(line[3])))
	}
	slices.Sort(found)
	return found
}

// fixtureLine は、設定の1行を「.ssh からの相対パス:行:キーワード」で表す。Windows の ssh は
// Include の glob の結果を区切りの混ざった表記で出すので、整えてから比べる。
func fixtureLine(t *testing.T, root, path, line, keyword string) string {
	t.Helper()
	relative, err := filepath.Rel(root, filepath.Clean(filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(relative) + ":" + line + ":" + keyword
}
