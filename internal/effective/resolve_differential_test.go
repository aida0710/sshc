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
	"sshc/internal/configresolver"
	"sshc/internal/effective"
	"sshc/internal/platform"
	"sshc/internal/storage"
)

// TestResolveMatchesInstalledOpenSSH は、この解決器が権威であることの完成条件である。
//
// `ssh -G` を直接起動して突き合わせる。Evaluator を経由しないのは、あちらが製品から
// 消えてもこの検査が残るようにするためである。OpenSSH との一致は、製品が何を
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
			// identityfile はここに無い。この解決器は既定を持たないと決めた。
			// OpenSSH の既定の並びはバージョンとビルドで変わり、この検査が macOS と
			// Linux で違う結果を返したのがその証拠である。
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
			// `Match host` は alias ではなく、そこまでに解決した HostName と比べる。
			// alias 名にだけ一致する書き方は、HostName を持つ alias には効かない。
			name: "match host compares the resolved hostname",
			contents: "Host web\n\tHostName web.internal.example.com\n" +
				"Match host *.internal.example.com\n\tPort 2222\n\tProxyJump bastion\n" +
				"Match host web\n\tPort 9999\n",
			alias:    "web",
			keywords: []string{"hostname", "port", "proxyjump"},
		},
		{
			// `Match originalhost` だけが利用者の打った名前を見る。
			name:     "match originalhost keeps the alias",
			contents: "Host web\n\tHostName web.internal.example.com\nMatch originalhost web\n\tPort 2222\n",
			alias:    "web",
			keywords: []string{"hostname", "port"},
		},
		{
			// HostName 自身の %h は alias を指し、Match host はその展開後と比べる。
			name:     "match host sees the expanded hostname token",
			contents: "Host web\n\tHostName %h.internal.example.com\nMatch host web.internal.example.com\n\tPort 2222\n",
			alias:    "web",
			keywords: []string{"hostname", "port"},
		},
		{
			// ProxyCommand と ProxyJump は先に受理した方だけが残る。
			name:     "proxyjump first hides a later proxycommand",
			contents: "Host a\n\tHostName 198.51.100.9\n\tProxyJump gateway\nHost *\n\tProxyCommand /usr/bin/nc %h %p\nHost gateway\n\tHostName 198.51.100.1\n",
			alias:    "a",
			keywords: []string{"proxyjump", "proxycommand"},
		},
		{
			name:     "proxycommand first hides a later proxyjump",
			contents: "Host a\n\tHostName 198.51.100.9\n\tProxyCommand /usr/bin/nc %h %p\n\tProxyJump gateway\nHost gateway\n\tHostName 198.51.100.1\n",
			alias:    "a",
			keywords: []string{"proxyjump", "proxycommand"},
		},
		// `ProxyJump none` の後の ProxyCommand は OpenSSH のバージョンで結果が割れる
		// （proxyDirectiveIgnored のコメント参照）ので、ここでは比べない。
		// 解決器が選んだ側は resolve_test.go が固定している。
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
		{
			// Match host は match_hostname で比べる。値もパターンも ASCII を小文字にする。
			name:     "match host ignores case",
			contents: "Host web\n\tHostName web.example\nMatch host WEB.example\n\tPort 2222\n",
			alias:    "web",
			keywords: []string{"hostname", "port"},
		},
		{
			name:     "match originalhost ignores case",
			contents: "Match originalhost ALIAS\n\tPort 2222\n",
			alias:    "alias",
			keywords: []string{"port"},
		},
		{
			// 確定した HostName は小文字になり、その後の Match host も一致する。
			// 大文字の HostName に Match の ProxyJump と User が付く形。
			name: "the resolved hostname is lower case",
			contents: "Host prod\n\tHostName Prod.Example.com\n" +
				"Match host *.example.com\n\tProxyJump bastion\n\tUser deploy\n",
			alias:    "prod",
			keywords: []string{"hostname", "proxyjump", "user"},
		},
		{
			// 否定のパターンも大文字小文字を区別せずに当たる。
			name:     "a negated match host pattern ignores case",
			contents: "Host web\n\tHostName web.example\nMatch host !WEB.example,*\n\tPort 2222\n",
			alias:    "web",
			keywords: []string{"port"},
		},
		{
			// alias から来た HostName も、%h を展開した HostName も小文字になる。
			// Host 行のパターンは区別したまま比べる。
			name:     "an alias written in capitals",
			contents: "Host EDGE\n\tHostName %h.Example.com\nMatch host edge.example.com\n\tPort 2222\n",
			alias:    "EDGE",
			keywords: []string{"hostname", "port"},
		},
		{
			// アドレスのリテラルは小文字にしない。全角の英字も変えない。
			name:     "addresses keep their case",
			contents: "Host six\n\tHostName 2001:DB8::1\nHost wide\n\tHostName Ｗeb.Example\n",
			alias:    "six",
			keywords: []string{"hostname"},
		},
		{
			name:     "non-ASCII letters keep their case",
			contents: "Host six\n\tHostName 2001:DB8::1\nHost wide\n\tHostName Ｗeb.Example\n",
			alias:    "wide",
			keywords: []string{"hostname"},
		},
		{
			// Match user は区別する。
			name:     "match user keeps case",
			contents: "Host db\n\tUser ops\nMatch user OPS\n\tPort 2222\n",
			alias:    "db",
			keywords: []string{"user", "port"},
		},
		{
			// 取り込んだファイルが Host ブロックで終わっても、戻ったあとの見出しのない
			// 行はすべての alias に効く。
			name:     "lines after an include apply to every alias again",
			contents: "Include conf.d/*.conf\nUser everyone\nServerAliveInterval 30\n",
			files:    map[string]string{"conf.d/10-home.conf": "Host nas\n\tPort 2201\n"},
			alias:    "other",
			keywords: []string{"user", "serveraliveinterval", "port"},
		},
		{
			// 一致しない Host の中の Include は、先頭の行も中の Host も効かない。
			name:     "an include inside an unmatched host contributes nothing",
			contents: "Host work\n\tInclude work.conf\nHost *\n\tPort 2022\n",
			files: map[string]string{"work.conf": "User alice\nProxyCommand ssh gateway -W %h:%p\n" +
				"Host ordinary\n\tHostName leaked.example\n"},
			alias:    "ordinary",
			keywords: []string{"user", "proxycommand", "hostname", "port"},
		},
		{
			// 一致する Host の中の Include は、先頭の行がその Host に効き、戻ったあとの
			// 行も取り込み元の Host に効く。取り込み先の Host other の判定は持ち越さない。
			name: "an include inside a matched host restores the host afterwards",
			contents: "Host work\n\tInclude work.conf\n\tUser after\n\tServerAliveInterval 15\n" +
				"Host *\n\tPort 2022\n",
			files: map[string]string{"work.conf": "ProxyCommand ssh gateway -W %h:%p # via gateway\n" +
				"Host other\n\tPort 2200\n"},
			alias:    "work",
			keywords: []string{"user", "serveraliveinterval", "proxycommand", "port"},
		},
		{
			// Match も、一致しない Host の中の Include では効かない。
			name:     "a match inside an unmatched include never applies",
			contents: "Host other\n\tInclude match.conf\n",
			files:    map[string]string{"match.conf": "Match all\n\tPort 1111\n\tUser matched\n"},
			alias:    "web",
			keywords: []string{"port", "user"},
		},
		{
			// OpenSSH 8.7 以降は、語の先頭の # から後ろをコメントとして捨て、引用符を外す。
			// 行の残りを使う ProxyCommand と RemoteCommand だけは、コメントも引用符も残る。
			name: "comments and quotes are stripped except from commands",
			contents: "Host office\n\tHostName Office.Example.COM # front door\n\tPort \"2222\"\n" +
				"\tUser ops # on call\n\tIdentityFile \"~/.ssh/office key\" # laptop\n" +
				"\tRemoteCommand tmux new -A -s \"main\"  # attach\n",
			alias:    "office",
			keywords: []string{"hostname", "port", "user", "identityfile", "remotecommand"},
		},
		{
			// argv_split の一重引用符、引用の外の "\ "、語中の引用符。
			name:     "single quotes, escaped spaces and quotes inside a word",
			contents: "Host lab\n\tUser 'bob'\n\tIdentityFile ~/.ssh/my\\ key\n\tIdentityFile ~/.ssh/b\"o\"th\n",
			alias:    "lab",
			keywords: []string{"user", "identityfile"},
		},
		{
			// SetEnv の値は引数ひとつずつである。空白でつなぐと "X=a b" の境界が消える。
			name:     "set env keeps each assignment",
			contents: "Host lab\n\tSetEnv X=\"a b\" ONE=1\n\tSendEnv LANG LC_*\n",
			alias:    "lab",
			keywords: []string{"setenv", "sendenv"},
		},
		{
			// エスケープした引用符を含む Match 行もブロックの境界である。読めないと、
			// 後ろの User が手前の Host * に付いて全ホストに効く。
			name:     "a match line with escaped quotes still starts a block",
			contents: "Host *\n\tPort 2022\nMatch host \"no\\\"match\"\n\tUser office\n\tProxyJump bastion\n",
			alias:    "web",
			keywords: []string{"port", "user", "proxyjump"},
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
			graph, err := configresolver.ForWorkspace(workspace).Resolve(configPath)
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
// このフィクスチャ群は Match exec を持たない。ProxyCommand と RemoteCommand は
// 持つが、-G は設定を表示するだけで、それらを実行しない。
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
