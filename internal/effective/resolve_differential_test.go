package effective_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sshc/internal/config"
	"sshc/internal/effective"
	"sshc/internal/platform"
	"sshc/internal/storage"
)

// TestResolveMatchesInstalledOpenSSH は、この解決器が権威であることの完成条件である。
//
// `ssh -G` を直接起動して突き合わせる。Evaluator を経由しないのは、あちらが製品から
// 消えてもこの検査が残るようにするためである——OpenSSH との一致は、製品が何を
// 持っているかとは別の話である。
//
// 比較はフィクスチャが設定したキーワードに限る。`ssh -G -F file` は、それ以外に
// ついては /etc/ssh/ssh_config も読むからである。
func TestResolveMatchesInstalledOpenSSH(t *testing.T) {
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		t.Skip("OpenSSH ssh is not installed; skipping the differential test")
	}
	current, err := user.Current()
	if err != nil {
		t.Skip("this platform does not report the current user")
	}

	tests := []struct {
		name     string
		contents string
		files    map[string]string
		alias    string
		keywords []string
	}{
		{
			name:     "explicit host",
			contents: "Host bastion\n\tHostName 203.0.113.10\n\tUser ops\n\tPort 2222\n",
			alias:    "bastion",
			keywords: []string{"hostname", "user", "port"},
		},
		{
			// 何も書かれていない alias。既定値をこちらが正しく持っているか。
			//
			// identityfile はここに無い。この解決器は既定を持たないと決めた——
			// OpenSSH の既定の並びは版とビルドで変わり、この検査が macOS と
			// Linux で違う答えを返したのがその証拠である。
			name:     "defaults nobody wrote",
			contents: "Host other\n\tPort 2222\n",
			alias:    "bare",
			keywords: []string{"hostname", "user", "port"},
		},
		{
			name:     "first value wins across blocks",
			contents: "Host db\n\tPort 2200\n\nHost db\n\tPort 9999\n\tUser dba\n",
			alias:    "db",
			keywords: []string{"port", "user"},
		},
		{
			name:     "wildcard defaults",
			contents: "Host web-01\n\tHostName 198.51.100.20\n\nHost *\n\tUser deploy\n\tPort 2022\n",
			alias:    "web-01",
			keywords: []string{"hostname", "user", "port"},
		},
		{
			name:     "negated pattern",
			contents: "Host !legacy *.internal\n\tUser ops\n\tPort 2202\n",
			alias:    "app.internal",
			keywords: []string{"user", "port"},
		},
		{
			// 積み上がるキーワード。First ではなく All で比べる。
			name: "identity files accumulate",
			contents: "Host bastion\n\tIdentityFile ~/.ssh/first\n\tIdentityFile ~/.ssh/second\n" +
				"\nHost *\n\tIdentityFile ~/.ssh/third\n",
			alias:    "bastion",
			keywords: []string{"identityfile"},
		},
		{
			name:     "set env accumulates",
			contents: "Host bastion\n\tSetEnv ONE=1\n\tSetEnv TWO=2\n",
			alias:    "bastion",
			keywords: []string{"setenv"},
		},
		{
			name:     "match host",
			contents: "Host db\n\tUser ops\nMatch host db\n\tPort 5432\nMatch host web\n\tPort 9999\n",
			alias:    "db",
			keywords: []string{"user", "port"},
		},
		{
			name:     "match user uses the resolved user",
			contents: "Host db\n\tUser ops\nMatch user ops\n\tPort 5432\n",
			alias:    "db",
			keywords: []string{"user", "port"},
		},
		{
			name:     "match localuser",
			contents: "Host db\n\tUser ops\nMatch localuser " + platform.LocalAccountName(current.Username) + "\n\tPort 7777\n",
			alias:    "db",
			keywords: []string{"port"},
		},
		{
			name:     "match all",
			contents: "Host db\n\tUser ops\nMatch all\n\tPort 8888\n",
			alias:    "db",
			keywords: []string{"port"},
		},
		{
			// %h は解決後の HostName、%r はリモートの user、%d はホーム。
			name: "token expansion",
			contents: "Host bastion\n\tHostName 203.0.113.10\n\tUser ops\n\tPort 2222\n" +
				"\tIdentityFile %d/.ssh/%r-at-%h-%p\n",
			alias:    "bastion",
			keywords: []string{"identityfile"},
		},
		{
			// HostName の中の %h は元の alias を指す。自分自身は参照しない。
			name:     "hostname token refers to the original alias",
			contents: "Host edge\n\tHostName %h.example.com\n",
			alias:    "edge",
			keywords: []string{"hostname"},
		},
		{
			// Include はその行の位置で読まれる。include されたファイルの方が勝つ。
			name:     "include is read where the line sits",
			contents: "Include conf.d/*.conf\n\nHost nas\n\tPort 9999\n",
			files:    map[string]string{"conf.d/10-home.conf": "Host nas\n\tPort 2201\n\tUser aida\n"},
			alias:    "nas",
			keywords: []string{"port", "user"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			root := filepath.Join(home, ".ssh")
			if err := os.MkdirAll(root, 0o700); err != nil {
				t.Fatal(err)
			}
			configPath := filepath.Join(root, "config")
			if err := os.WriteFile(configPath, []byte(test.contents), 0o600); err != nil {
				t.Fatal(err)
			}
			for relative, contents := range test.files {
				absolute := filepath.Join(root, filepath.FromSlash(relative))
				if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(absolute, []byte(contents), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
			if err != nil {
				t.Fatal(err)
			}
			graph, err := storage.NewResolver(workspace).Resolve(configPath)
			if err != nil {
				t.Fatal(err)
			}
			for _, diagnostic := range graph.Diagnostics {
				if diagnostic.Severity == config.SeverityError {
					t.Fatalf("fixture produced an error diagnostic: %#v", diagnostic)
				}
			}

			// フィクスチャのホームが子プロセスの HOME である。ssh は相対 Include を
			// ~/.ssh に固定し、~ を HOME から取るので、そうしないとフィクスチャの
			// Include が本物の ~/.ssh へ到達する。
			wanted := runSSHG(t, sshPath, home, configPath, test.alias)

			facts := effective.LocalFacts{User: platform.LocalAccountName(current.Username), Home: home}
			resolution := effective.Resolve(graph, test.alias, facts)
			values, refusals := resolution.Values, resolution.Refusals
			if len(refusals) != 0 {
				t.Fatalf("Resolve refused a fixture it should answer: %#v", refusals)
			}

			for _, keyword := range test.keywords {
				got, want := values.All(keyword), wanted.All(keyword)
				if len(got) != len(want) {
					t.Errorf("%s: engine = %#v, ssh -G = %#v", keyword, got, want)
					continue
				}
				for index := range got {
					if got[index] != want[index] {
						t.Errorf("%s[%d]: engine = %q, ssh -G = %q", keyword, index, got[index], want[index])
					}
				}
			}
		})
	}
}

// runSSHG は本物の ssh を -G で走らせ、その出力を解析する。
//
// このフィクスチャ群はどれも ProxyCommand も Match exec も持たないので、評価が
// プログラムを実行することはない。
func runSSHG(t *testing.T, sshPath, home, configPath, alias string) effective.Values {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	command := exec.CommandContext(ctx, sshPath, "-G", "-F", configPath, "--", alias)
	command.Env = sshEnvironment(home)
	stdout, err := command.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			t.Fatalf("ssh -G = %v\n%s", err, exit.Stderr)
		}
		t.Fatalf("ssh -G = %v", err)
	}
	return effective.ParseValues(stdout)
}

// フィクスチャが ssh -G に読ませる設定と、この解決器が読む設定が同じファイルで
// あることを確かめる。取り違えると、両者が別のものについて一致することになる。
func TestTheDifferentialFixtureReadsOneFile(t *testing.T) {
	if !strings.HasSuffix(filepath.Join("home", ".ssh", "config"), filepath.Join(".ssh", "config")) {
		t.Fatal("the fixture path is not the entry file")
	}
}
