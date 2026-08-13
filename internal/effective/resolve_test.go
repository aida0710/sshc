package effective_test

import (
	"testing"

	"sshc/internal/effective"
)

func resolveFacts() effective.LocalFacts {
	return effective.LocalFacts{User: "aida", Home: testHome, Hostname: "mac.local", UID: "501"}
}

func TestResolveAnswersWithTheValuesTheConnectionUses(t *testing.T) {
	graph := graphFor(t, map[string]string{
		testConfig: "Host bastion\n" +
			"\tHostName 203.0.113.10\n" +
			"\tUser ops\n" +
			"\nHost *\n" +
			"\tPort 2222\n",
	})

	resolution := effective.Resolve(graph, "bastion", resolveFacts())
	values, refusals := resolution.Values, resolution.Refusals
	if len(refusals) != 0 {
		t.Fatalf("refusals = %#v", refusals)
	}
	if values.First("hostname") != "203.0.113.10" || values.First("user") != "ops" {
		t.Errorf("values = %#v", values.Entries)
	}
	// catch-all の値も届く。最初の値が勝つ規則は、ブロックをまたいでも同じである。
	if values.First("port") != "2222" {
		t.Errorf("port = %q, want the wildcard block's value", values.First("port"))
	}
}

func TestResolveFillsOnlyTheDefaultsItOwns(t *testing.T) {
	graph := graphFor(t, map[string]string{testConfig: "Host bare\n\tCompression yes\n"})

	resolution := effective.Resolve(graph, "bare", resolveFacts())
	values, refusals := resolution.Values, resolution.Refusals
	if len(refusals) != 0 {
		t.Fatalf("refusals = %#v", refusals)
	}
	if values.First("hostname") != "bare" {
		t.Errorf("hostname = %q, want the alias", values.First("hostname"))
	}
	if values.First("user") != "aida" {
		t.Errorf("user = %q, want the local account", values.First("user"))
	}
	if values.First("port") != "22" {
		t.Errorf("port = %q", values.First("port"))
	}
	// 書かれている他のキーワードはそのまま返る。
	if values.First("compression") != "yes" {
		t.Errorf("compression = %q", values.First("compression"))
	}
	// 誰も書いていないキーワードは現れない。OpenSSH の既定値表は持たない。
	if _, present := values.Entries["serveraliveinterval"]; present {
		t.Errorf("a keyword nobody wrote must not appear: %#v", values.Entries)
	}
}

// 積み上がるキーワードは、書かれた順にすべて残る。
func TestResolveAccumulatesTheKeywordsOpenSSHAccumulates(t *testing.T) {
	graph := graphFor(t, map[string]string{
		testConfig: "Host bastion\n" +
			"\tIdentityFile ~/.ssh/a\n" +
			"\tIdentityFile ~/.ssh/b\n" +
			"\tUser first\n" +
			"\tUser second\n",
	})

	values := effective.Resolve(graph, "bastion", resolveFacts()).Values
	if got := values.All("identityfile"); len(got) != 2 || got[0] != "~/.ssh/a" || got[1] != "~/.ssh/b" {
		t.Errorf("identityfile = %#v, want both in order", got)
	}
	if got := values.All("user"); len(got) != 1 || got[0] != "first" {
		t.Errorf("user = %#v, want only the first", got)
	}
}

func TestResolveEvaluatesMatchBlocks(t *testing.T) {
	graph := graphFor(t, map[string]string{
		testConfig: "Host db\n" +
			"\tUser ops\n" +
			"Match host db user ops\n" +
			"\tPort 5432\n" +
			"Match host web\n" +
			"\tPort 9999\n",
	})

	resolution := effective.Resolve(graph, "db", resolveFacts())
	values, refusals := resolution.Values, resolution.Refusals
	if len(refusals) != 0 {
		t.Fatalf("refusals = %#v", refusals)
	}
	// 一致した Match は値を寄与する。
	if values.First("port") != "5432" {
		t.Errorf("port = %q, want the matching block's value", values.First("port"))
	}
}

// 解決器は何も実行しない。exec の結果に依存する設定は、値を推測しない。
func TestResolveRefusesWhatItWillNotEvaluate(t *testing.T) {
	for _, test := range []struct {
		name     string
		contents string
		want     string
	}{
		{
			"match exec",
			"Match exec \"true\"\n\tUser hidden\n\nHost bastion\n\tUser ops\n",
			effective.RefusalMatchExec,
		},
		{
			"match final",
			"Match final\n\tUser late\n\nHost bastion\n\tUser ops\n",
			effective.RefusalMatchFinal,
		},
		{
			"canonicalize",
			"Host bastion\n\tCanonicalizeHostname yes\n\tUser ops\n",
			effective.RefusalCanonicalize,
		},
	} {
		graph := graphFor(t, map[string]string{testConfig: test.contents})
		resolution := effective.Resolve(graph, "bastion", resolveFacts())
		values, refusals := resolution.Values, resolution.Refusals

		if len(refusals) == 0 || refusals[0].Code != test.want {
			t.Errorf("%s: refusals = %#v, want %s", test.name, refusals, test.want)
			continue
		}
		// 部分的な答えを黙って返さない。
		if len(values.Entries) != 0 {
			t.Errorf("%s: a refused configuration carried values: %#v", test.name, values.Entries)
		}
	}
}

// CanonicalizeHostname no は既定なので、書いてあっても拒む理由にならない。
func TestResolveAcceptsCanonicalisationTurnedOff(t *testing.T) {
	graph := graphFor(t, map[string]string{
		testConfig: "Host bastion\n\tCanonicalizeHostname no\n\tUser ops\n",
	})

	resolution := effective.Resolve(graph, "bastion", resolveFacts())
	values, refusals := resolution.Values, resolution.Refusals
	if len(refusals) != 0 {
		t.Fatalf("refusals = %#v", refusals)
	}
	if values.First("user") != "ops" {
		t.Errorf("user = %q", values.First("user"))
	}
}

func TestResolveExpandsTokensAfterTheValuesAreKnown(t *testing.T) {
	graph := graphFor(t, map[string]string{
		testConfig: "Host bastion\n" +
			"\tHostName %h.example.com\n" +
			"\tUser ops\n" +
			"\tPort 2222\n" +
			"\tIdentityFile %d/.ssh/%r@%h:%p\n",
	})

	resolution := effective.Resolve(graph, "bastion", resolveFacts())
	values, refusals := resolution.Values, resolution.Refusals
	if len(refusals) != 0 {
		t.Fatalf("refusals = %#v", refusals)
	}
	// HostName の中の %h は元の alias を指す。自分自身は参照しない。
	if values.First("hostname") != "bastion.example.com" {
		t.Errorf("hostname = %q", values.First("hostname"))
	}
	// **IdentityFile のトークンは展開しない。** ssh -G もそうする——OpenSSH が
	// それを展開するのは接続する瞬間であって、設定を読み終えた時点ではない。
	// ここで展開すると、設定について報告する値が ssh の報告とずれる。
	if values.First("identityfile") != "%d/.ssh/%r@%h:%p" {
		t.Errorf("identityfile = %q, want it unexpanded", values.First("identityfile"))
	}
}

// 展開するキーワードの中に展開できないトークンがあれば、黙って残さない。その
// 文字列はホスト名としてそのまま使われてしまう。
func TestResolveRefusesATokenItCannotExpand(t *testing.T) {
	graph := graphFor(t, map[string]string{
		testConfig: "Host bastion\n\tHostName %C.example.com\n",
	})

	resolution := effective.Resolve(graph, "bastion", resolveFacts())
	values, refusals := resolution.Values, resolution.Refusals
	if len(refusals) != 1 || refusals[0].Code != effective.RefusalUnknownToken {
		t.Fatalf("refusals = %#v", refusals)
	}
	if len(values.Entries) != 0 {
		t.Errorf("a refused configuration carried values: %#v", values.Entries)
	}
}

// トークンを受け取らないキーワードの値は、% を含んでいてもそのまま返る。
func TestResolveLeavesPercentAloneWhereOpenSSHDoes(t *testing.T) {
	graph := graphFor(t, map[string]string{
		testConfig: "Host bastion\n\tSetEnv PROMPT=100%%\n",
	})

	resolution := effective.Resolve(graph, "bastion", resolveFacts())
	values, refusals := resolution.Values, resolution.Refusals
	if len(refusals) != 0 {
		t.Fatalf("refusals = %#v", refusals)
	}
	if values.First("setenv") != "PROMPT=100%%" {
		t.Errorf("setenv = %q, want it untouched", values.First("setenv"))
	}
}
