package effective_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"sshc/internal/effective"
)

// ParallelCluster の案内する形。Host の中から取り込んだファイルの ProxyCommand は、
// その Host にだけ効く。接続に使う ProxyCommand と、確認に出す ProxyCommand が
// 同じでなければ、確認なしでコマンドが走る。
func TestScanForAliasListsTheProxyCommandThatResolveUses(t *testing.T) {
	graph := graphFor(t, map[string]string{
		testConfig: "Host pcluster-head\n\tInclude pcluster.conf\n\tUser after\n" +
			"Host *\n\tServerAliveInterval 30\n",
		filepath.Join(testRoot, "pcluster.conf"): "ProxyCommand ssh gateway -W %h:%p\n" +
			"Host other\n\tPort 2200\n",
	})

	for _, alias := range []string{"pcluster-head", "ordinary", "other"} {
		resolution := effective.Resolve(graph, alias, resolveFacts())
		if len(resolution.Refusals) != 0 {
			t.Fatalf("%s: refusals = %#v", alias, resolution.Refusals)
		}
		var scanned []string
		for _, directive := range effective.ScanForAlias(graph, alias).Directives {
			if directive.Keyword == "ProxyCommand" {
				scanned = append(scanned, directive.Command)
			}
		}
		if got := resolution.Values.All("proxycommand"); !reflect.DeepEqual(got, scanned) {
			t.Errorf("%s: Resolve proxycommand = %q, ScanForAlias = %q", alias, got, scanned)
		}
	}
}

// 取り込み元の Host に一致しない alias には、取り込み先の Host も一致しない。
// 戻ったあとは Include 行の時点の状態に戻る。
func TestResolveCarriesTheIncludingBlockIntoAndOutOfAnInclude(t *testing.T) {
	graph := graphFor(t, map[string]string{
		testConfig: "Host work\n\tInclude work.conf\n\tUser after\n" +
			"Host *\n\tPort 2022\n",
		filepath.Join(testRoot, "work.conf"): "IdentityFile ~/.ssh/work\n" +
			"Host other\n\tHostName other.example\n" +
			"Host ordinary\n\tUser bob\n",
	})

	work := effective.Resolve(graph, "work", resolveFacts()).Values
	if work.First("user") != "after" || work.First("identityfile") != "~/.ssh/work" || work.First("port") != "2022" {
		t.Errorf("work = %#v, want the including block restored after the Include", work.Entries)
	}
	ordinary := effective.Resolve(graph, "ordinary", resolveFacts()).Values
	if ordinary.First("user") != "aida" || len(ordinary.All("identityfile")) != 0 {
		t.Errorf("ordinary = %#v, want nothing from an Include inside Host work", ordinary.Entries)
	}
}

// 最後が Host ブロックで終わるファイルを取り込んでも、戻ったあとの見出しのない行は
// すべての alias に効く。
func TestResolveReturnsToTheGlobalBlockAfterAnInclude(t *testing.T) {
	graph := graphFor(t, map[string]string{
		testConfig: "Include conf.d/*.conf\nUser everyone\n",
		filepath.Join(testRoot, "conf.d", "a.conf"): "Host nas\n\tPort 2201\n",
	})

	if user := effective.Resolve(graph, "other", resolveFacts()).Values.First("user"); user != "everyone" {
		t.Errorf("user = %q, want the global line after the Include", user)
	}
}

// 一致しないブロックの中の Include から読まれた Match exec も、OpenSSH は実行する。
// 値は決まるので Resolve は断らないが、実行されうるコマンドとしては挙げる。
func TestMatchExecInsideAnUnmatchedIncludeIsListedButDoesNotBlockResolve(t *testing.T) {
	graph := graphFor(t, map[string]string{
		testConfig:                        "Host other\n\tInclude m.conf\n",
		filepath.Join(testRoot, "m.conf"): "Match exec \"test -e /tmp/x\"\n\tPort 1111\n",
	})

	resolution := effective.Resolve(graph, "web", resolveFacts())
	if len(resolution.Refusals) != 0 || resolution.Values.First("port") != "22" {
		t.Fatalf("resolution = %#v", resolution)
	}
	report := effective.ScanForAlias(graph, "web")
	if len(report.Directives) != 1 || report.Directives[0].Command != "test -e /tmp/x" {
		t.Fatalf("directives = %#v, want the Match exec OpenSSH runs", report.Directives)
	}
}

// Resolve の Notes と Project の Complexities は、Host ブロックについて同じ印を出す。
func TestResolveAndProjectMarkTheSameHostBlocks(t *testing.T) {
	graph := graphFor(t, map[string]string{
		testConfig: "Host web\n\tUser a\nHost web !legacy\n\tPort 1\nHost w*\n\tPort 2\n" +
			"Include missing/*.conf\n",
	})

	codes := func(complexities []effective.Complexity) []string {
		var listed []string
		for _, complexity := range complexities {
			listed = append(listed, complexity.Code+"@"+filepath.Base(complexity.Path))
		}
		return listed
	}
	resolved := codes(effective.Resolve(graph, "web", resolveFacts()).Notes)
	projected := codes(effective.Project(graph, "web").Complexities)
	if !reflect.DeepEqual(resolved, projected) {
		t.Fatalf("Resolve notes = %q, Project complexities = %q", resolved, projected)
	}
	if len(resolved) != 4 {
		t.Fatalf("notes = %q, want duplicate, negated, wildcard and unresolved include", resolved)
	}
}
