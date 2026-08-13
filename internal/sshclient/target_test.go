package sshclient_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"sshc/internal/effective"
	"sshc/internal/sshclient"
)

// valuesFor は、解決器の答えを手で組み立てる。
//
// 設定を読むのはこのパッケージの仕事ではないので、フィクスチャも設定ではなく
// 「解決の答え」である。
func valuesFor(entries map[string][]string) effective.Values {
	values := effective.Values{Entries: map[string][]string{}}
	for keyword, list := range entries {
		values.Keywords = append(values.Keywords, keyword)
		values.Entries[keyword] = list
	}
	return values
}

func resolverFor(table map[string]map[string][]string) sshclient.Resolver {
	return func(alias string) (effective.Values, error) {
		entries, known := table[alias]
		if !known {
			return effective.Values{}, errors.New("no such alias: " + alias)
		}
		return valuesFor(entries), nil
	}
}

func TestNewTargetTakesTheValuesTheResolverDecided(t *testing.T) {
	resolve := resolverFor(map[string]map[string][]string{
		"bastion": {
			"hostname":     {"203.0.113.10"},
			"port":         {"2222"},
			"user":         {"ops"},
			"identityfile": {"~/.ssh/first", "/etc/keys/second"},
			"setenv":       {"ONE=1 TWO=2"},
		},
	})

	target, notices, err := sshclient.NewTarget("bastion", resolve, "/home/aida")
	if err != nil {
		t.Fatal(err)
	}
	if len(notices) != 0 {
		t.Errorf("notices = %#v", notices)
	}
	if target.Address() != "203.0.113.10:2222" || target.User != "ops" {
		t.Errorf("target = %+v", target)
	}
	// ~ はここで解く。ssh -G は ~ を残すので、解決器の答えには残っている。
	want := []string{"/home/aida/.ssh/first", "/etc/keys/second"}
	if len(target.Identities) != 2 || target.Identities[0] != want[0] || target.Identities[1] != want[1] {
		t.Errorf("identities = %#v, want %#v", target.Identities, want)
	}
	if len(target.SetEnv) != 2 || target.SetEnv[0] != (sshclient.EnvVar{Name: "ONE", Value: "1"}) {
		t.Errorf("setenv = %#v", target.SetEnv)
	}
}

// 何も書かれていない設定でも接続はできる。既定を持つのは解決器なので、
// ここに来る時点で hostname も port も user も決まっている。
func TestNewTargetRefusesWhenNoHostNameWasDecided(t *testing.T) {
	resolve := resolverFor(map[string]map[string][]string{"bare": {}})
	if _, _, err := sshclient.NewTarget("bare", resolve, "/home/aida"); !errors.Is(err, sshclient.ErrNoHostName) {
		t.Fatalf("NewTarget = %v, want ErrNoHostName", err)
	}
}

// **接続のためにプログラムを起こさない。** B1 が Match exec について決めたことと
// 同じ理由で、ProxyCommand を持つ設定では接続そのものを組み立てない。
func TestNewTargetRefusesProxyCommand(t *testing.T) {
	resolve := resolverFor(map[string]map[string][]string{
		"jump": {"hostname": {"198.51.100.9"}, "proxycommand": {"/usr/bin/nc %h %p"}},
	})
	if _, _, err := sshclient.NewTarget("jump", resolve, "/home/aida"); !errors.Is(err, sshclient.ErrProxyCommand) {
		t.Fatalf("NewTarget = %v, want ErrProxyCommand", err)
	}
}

// ProxyCommand none は「使わない」という指定なので、断る理由にならない。
func TestNewTargetAcceptsProxyCommandTurnedOff(t *testing.T) {
	resolve := resolverFor(map[string]map[string][]string{
		"direct": {"hostname": {"198.51.100.9"}, "proxycommand": {"none"}},
	})
	if _, _, err := sshclient.NewTarget("direct", resolve, "/home/aida"); err != nil {
		t.Fatalf("NewTarget = %v", err)
	}
}

func TestNewTargetExpandsTheProxyJumpChainInOrder(t *testing.T) {
	resolve := resolverFor(map[string]map[string][]string{
		"inner": {"hostname": {"10.0.0.5"}, "user": {"deep"}, "proxyjump": {"edge"}},
		"edge":  {"hostname": {"198.51.100.1"}, "user": {"gate"}, "port": {"2200"}},
		"final": {"hostname": {"10.0.0.9"}, "proxyjump": {"inner"}},
	})

	target, _, err := sshclient.NewTarget("final", resolve, "/home/aida")
	if err != nil {
		t.Fatal(err)
	}
	if len(target.Jump) != 1 || target.Jump[0].Alias != "inner" {
		t.Fatalf("jump = %#v", target.Jump)
	}
	inner := target.Jump[0]
	if len(inner.Jump) != 1 || inner.Jump[0].Address() != "198.51.100.1:2200" {
		t.Fatalf("nested jump = %#v", inner.Jump)
	}
}

// リストに書かれた user と port は、そのホップ自身の設定に勝つ。
func TestAJumpListOverridesTheHopOwnUserAndPort(t *testing.T) {
	resolve := resolverFor(map[string]map[string][]string{
		"target": {"hostname": {"10.0.0.9"}, "proxyjump": {"someone@edge:2022"}},
		"edge":   {"hostname": {"198.51.100.1"}, "user": {"gate"}, "port": {"2200"}},
	})

	target, _, err := sshclient.NewTarget("target", resolve, "/home/aida")
	if err != nil {
		t.Fatal(err)
	}
	hop := target.Jump[0]
	if hop.User != "someone" || hop.Port != "2022" {
		t.Errorf("hop = %+v, want the list's user and port", hop)
	}
}

func TestAProxyJumpCycleStopsAtTheDepthLimit(t *testing.T) {
	resolve := resolverFor(map[string]map[string][]string{
		"a": {"hostname": {"10.0.0.1"}, "proxyjump": {"b"}},
		"b": {"hostname": {"10.0.0.2"}, "proxyjump": {"a"}},
	})
	if _, _, err := sshclient.NewTarget("a", resolve, "/home/aida"); !errors.Is(err, sshclient.ErrJumpDepth) {
		t.Fatalf("NewTarget = %v, want ErrJumpDepth", err)
	}
}

// ProxyJump none は連鎖を持たないという指定である。
func TestProxyJumpNoneLeavesNoChain(t *testing.T) {
	resolve := resolverFor(map[string]map[string][]string{
		"plain": {"hostname": {"10.0.0.9"}, "proxyjump": {"none"}},
	})
	target, _, err := sshclient.NewTarget("plain", resolve, "/home/aida")
	if err != nil {
		t.Fatal(err)
	}
	if len(target.Jump) != 0 {
		t.Errorf("jump = %#v", target.Jump)
	}
}

// 接続はするが honour しないものは notice になる。転送が無いことを理由に
// 接続そのものを断ると、転送を使っていない日にも繋がらなくなる。
func TestUnhonouredKeywordsBecomeNoticesRatherThanRefusals(t *testing.T) {
	resolve := resolverFor(map[string]map[string][]string{
		"work": {
			"hostname":     {"10.0.0.9"},
			"localforward": {"8080 127.0.0.1:80"},
			"forwardagent": {"yes"},
			"forwardx11":   {"no"},
		},
	})

	target, notices, err := sshclient.NewTarget("work", resolve, "/home/aida")
	if err != nil {
		t.Fatal(err)
	}
	if target.HostName != "10.0.0.9" {
		t.Errorf("the connection was not built: %+v", target)
	}
	found := map[string]bool{}
	for _, notice := range notices {
		found[notice.Keyword] = true
	}
	if !found["localforward"] || !found["forwardagent"] {
		t.Errorf("notices = %#v", notices)
	}
	// no と書いてあるものは、無いことが望みなので黙っている。
	if found["forwardx11"] {
		t.Error("ForwardX11 no produced a notice about a feature the user turned off")
	}
}

func TestMethodOrderFollowsPreferredAuthentications(t *testing.T) {
	resolve := resolverFor(map[string]map[string][]string{
		"host": {
			"hostname":                     {"10.0.0.9"},
			"preferredauthentications":     {"password,publickey"},
			"kbdinteractiveauthentication": {"no"},
		},
	})
	target, _, err := sshclient.NewTarget("host", resolve, "/home/aida")
	if err != nil {
		t.Fatal(err)
	}
	order := target.Methods.Order()
	if len(order) != 2 || order[0] != "password" || order[1] != "publickey" {
		t.Errorf("order = %#v", order)
	}
}

// 何も書かれていなければ OpenSSH の既定順である。gssapi と hostbased は
// このクライアントに無いので現れない。
func TestTheDefaultMethodOrderIsOpenSSHs(t *testing.T) {
	order := sshclient.DefaultMethods().Order()
	want := []string{"publickey", "keyboard-interactive", "password"}
	if len(order) != len(want) {
		t.Fatalf("order = %#v, want %#v", order, want)
	}
	for index := range want {
		if order[index] != want[index] {
			t.Fatalf("order = %#v, want %#v", order, want)
		}
	}
}

func TestNumericValuesBecomeDurations(t *testing.T) {
	resolve := resolverFor(map[string]map[string][]string{
		"host": {
			"hostname":            {"10.0.0.9"},
			"serveraliveinterval": {"30"},
			"serveralivecountmax": {"2"},
			"connecttimeout":      {"nonsense"},
		},
	})
	target, _, err := sshclient.NewTarget("host", resolve, "/home/aida")
	if err != nil {
		t.Fatal(err)
	}
	if target.KeepAlive != 30*time.Second || target.KeepAliveMax != 2 {
		t.Errorf("keepalive = %v/%d", target.KeepAlive, target.KeepAliveMax)
	}
	// 読めない値は既定へ戻す。設定の数字ひとつで接続できなくなる理由はない。
	if target.Timeout != 0 {
		t.Errorf("timeout = %v", target.Timeout)
	}
}

// **「まだ無い」と「無い」を区別して書く。** 永久に無いものを、来週来るかの
// ように言わない。B4 で落とすと決めたキーワードは、その理由を添えて notice を
// 出し続ける——黙って無視すると、書いた設定が効いていないことに気づけない。
func TestDroppedKeywordsSayWhyRatherThanPromisingThemLater(t *testing.T) {
	entries := map[string][]string{"hostname": {"10.0.0.9"}}
	dropped := []string{
		"remoteforward", "forwardx11", "controlmaster", "controlpath",
		"localcommand", "certificatefile", "sendenv",
	}
	for _, keyword := range dropped {
		entries[keyword] = []string{"something the user wrote"}
	}
	resolve := resolverFor(map[string]map[string][]string{"work": entries})

	_, notices, err := sshclient.NewTarget("work", resolve, "/home/aida")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, notice := range notices {
		seen[notice.Keyword] = notice.Detail
	}
	for _, keyword := range dropped {
		detail, found := seen[keyword]
		if !found {
			t.Errorf("%s was dropped silently", keyword)
			continue
		}
		if strings.Contains(detail, "not implemented yet") {
			t.Errorf("%s says it is coming later, but it was dropped: %q", keyword, detail)
		}
	}
}

// まだ無いものは、そう言ってよい。
func TestKeywordsStillToComeSaySo(t *testing.T) {
	resolve := resolverFor(map[string]map[string][]string{
		"work": {
			"hostname":       {"10.0.0.9"},
			"localforward":   {"8080 127.0.0.1:80"},
			"dynamicforward": {"1080"},
			"forwardagent":   {"yes"},
		},
	})
	_, notices, err := sshclient.NewTarget("work", resolve, "/home/aida")
	if err != nil {
		t.Fatal(err)
	}
	if len(notices) != 3 {
		t.Fatalf("notices = %#v", notices)
	}
	for _, notice := range notices {
		if !strings.Contains(notice.Detail, "not implemented yet") {
			t.Errorf("%s = %q, want it to say it is still to come", notice.Keyword, notice.Detail)
		}
	}
}
