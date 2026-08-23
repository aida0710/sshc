package sshclient_test

import (
	"errors"
	"path/filepath"
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
			"hostname": {"203.0.113.10"},
			"port":     {"2222"},
			"user":     {"ops"},
			// 設定に書かれる綴りはスラッシュ区切りである。解くのはこの下。
			"identityfile": {"~/.ssh/first", testOutsideKey},
			"setenv":       {"ONE=1 TWO=2"},
		},
	})

	target, err := sshclient.NewTarget("bastion", resolve, testHome)
	notices := target.Notices
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
	// **ホームの外にある二つ目は、継ぎ足されず絶対のまま残らなければならない。**
	want := []string{filepath.Join(testHome, ".ssh", "first"), testOutsideKey}
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
	if _, err := sshclient.NewTarget("bare", resolve, testHome); !errors.Is(err, sshclient.ErrNoHostName) {
		t.Fatalf("NewTarget = %v, want ErrNoHostName", err)
	}
}

// **ProxyCommand は接続の一部として運ぶ。** かつてここは断っていた。
//
// トークンはこの時点で展開する。解決器は生のまま返す——`ssh -G` がそうする
// からで、OpenSSH が展開するのは繋ぐ瞬間である。**展開しないと、`%h` を
// 持ったままの綴りがそのままプログラムの引数になる。**
func TestNewTargetCarriesProxyCommandWithItsTokensExpanded(t *testing.T) {
	resolve := resolverFor(map[string]map[string][]string{
		"jump": {
			"hostname":     {"198.51.100.9"},
			"port":         {"2222"},
			"user":         {"ops"},
			"proxycommand": {"/usr/bin/nc %h %p"},
		},
	})
	target, err := sshclient.NewTarget("jump", resolve, testHome)
	if err != nil {
		t.Fatalf("NewTarget = %v", err)
	}
	if target.ProxyCommand != "/usr/bin/nc 198.51.100.9 2222" {
		t.Errorf("ProxyCommand = %q", target.ProxyCommand)
	}
}

// **ProxyJump と一緒には書けない。** どちらも「どうやって届くか」を決めるので、
// 両方書いた人は二つの違う答えを書いている。ssh も断る
// （"inconsistent options: ProxyCommand+ProxyJump"）。
func TestNewTargetRefusesProxyCommandTogetherWithProxyJump(t *testing.T) {
	resolve := resolverFor(map[string]map[string][]string{
		"both": {
			"hostname":     {"198.51.100.9"},
			"proxycommand": {"/usr/bin/nc %h %p"},
			"proxyjump":    {"gateway"},
		},
		"gateway": {"hostname": {"198.51.100.1"}},
	})
	if _, err := sshclient.NewTarget("both", resolve, testHome); !errors.Is(err, sshclient.ErrProxyCommandWithJump) {
		t.Fatalf("NewTarget = %v, want ErrProxyCommandWithJump", err)
	}
}

// **ProxyJump none は「使わない」なので、衝突ではない。**
func TestNewTargetAcceptsProxyCommandWhenProxyJumpIsOff(t *testing.T) {
	resolve := resolverFor(map[string]map[string][]string{
		"one": {
			"hostname":     {"198.51.100.9"},
			"proxycommand": {"/usr/bin/nc %h %p"},
			"proxyjump":    {"none"},
		},
	})
	target, err := sshclient.NewTarget("one", resolve, testHome)
	if err != nil {
		t.Fatalf("NewTarget = %v", err)
	}
	if target.ProxyCommand == "" {
		t.Error("ProxyCommand was dropped")
	}
}

// ProxyCommand none は「使わない」という指定なので、断る理由にならない。
func TestNewTargetAcceptsProxyCommandTurnedOff(t *testing.T) {
	resolve := resolverFor(map[string]map[string][]string{
		"direct": {"hostname": {"198.51.100.9"}, "proxycommand": {"none"}},
	})
	target, err := sshclient.NewTarget("direct", resolve, testHome)
	if err != nil {
		t.Fatalf("NewTarget = %v", err)
	}
	if target.ProxyCommand != "" {
		t.Errorf("ProxyCommand = %q, want none", target.ProxyCommand)
	}
}

func TestNewTargetExpandsTheProxyJumpChainInOrder(t *testing.T) {
	resolve := resolverFor(map[string]map[string][]string{
		"inner": {"hostname": {"10.0.0.5"}, "user": {"deep"}, "proxyjump": {"edge"}},
		"edge":  {"hostname": {"198.51.100.1"}, "user": {"gate"}, "port": {"2200"}},
		"final": {"hostname": {"10.0.0.9"}, "proxyjump": {"inner"}},
	})

	target, err := sshclient.NewTarget("final", resolve, testHome)
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

	target, err := sshclient.NewTarget("target", resolve, testHome)
	if err != nil {
		t.Fatal(err)
	}
	hop := target.Jump[0]
	if hop.User != "someone" || hop.Port != "2022" {
		t.Errorf("hop = %+v, want the list's user and port", hop)
	}
}

// ProxyJump のトークンは、繋ぐ側が展開する。
//
// **`ssh -G` は生のまま報告する。** OpenSSH がこれを展開するのは繋ぐ瞬間だから
// であり、解決器もそれに倣っている。だから展開はここでしか起きない。展開しないと
// `%r@gateway` は「%r という名前の利用者」への認証になり、publickey は通らず、
// 残る方式も無いまま握手が終わる——実際そうなっていた。
//
// %r が指すのは最終的な行き先の利用者であって、手前のホップのそれではない。
func TestProxyJumpTokensAreExpandedAgainstTheFinalDestination(t *testing.T) {
	resolve := resolverFor(map[string]map[string][]string{
		"far": {
			"hostname":  {"qes02"},
			"user":      {"someone"},
			"port":      {"2022"},
			"proxyjump": {"%r@gateway.example"},
		},
		"gateway.example": {"hostname": {"gateway.example"}, "user": {"nobody"}},
	})

	target, err := sshclient.NewTarget("far", resolve, testHome)
	if err != nil {
		t.Fatal(err)
	}
	if len(target.Jump) != 1 {
		t.Fatalf("jump = %#v", target.Jump)
	}
	if target.Jump[0].User != "someone" {
		t.Errorf("hop user = %q, want the final destination's user", target.Jump[0].User)
	}
}

// ProxyJump が受け取るのは %%、%h、%n、%p、%r の 5 つだけである。
//
// 展開できないものを黙って残さない。その文字列はユーザー名やホスト名として
// そのまま使われる。
func TestProxyJumpRefusesATokenOpenSSHDoesNotAllowThere(t *testing.T) {
	resolve := resolverFor(map[string]map[string][]string{
		"far": {"hostname": {"10.0.0.9"}, "proxyjump": {"%u@gateway.example"}},
	})
	if _, err := sshclient.NewTarget("far", resolve, testHome); err == nil {
		t.Fatal("NewTarget accepted a token ProxyJump does not take")
	}
}

func TestAProxyJumpCycleStopsAtTheDepthLimit(t *testing.T) {
	resolve := resolverFor(map[string]map[string][]string{
		"a": {"hostname": {"10.0.0.1"}, "proxyjump": {"b"}},
		"b": {"hostname": {"10.0.0.2"}, "proxyjump": {"a"}},
	})
	if _, err := sshclient.NewTarget("a", resolve, testHome); !errors.Is(err, sshclient.ErrJumpDepth) {
		t.Fatalf("NewTarget = %v, want ErrJumpDepth", err)
	}
}

// ProxyJump none は連鎖を持たないという指定である。
func TestProxyJumpNoneLeavesNoChain(t *testing.T) {
	resolve := resolverFor(map[string]map[string][]string{
		"plain": {"hostname": {"10.0.0.9"}, "proxyjump": {"none"}},
	})
	target, err := sshclient.NewTarget("plain", resolve, testHome)
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
			"hostname":      {"10.0.0.9"},
			"remoteforward": {"8080 127.0.0.1:80"},
			"controlmaster": {"auto"},
			"forwardx11":    {"no"},
		},
	})

	target, err := sshclient.NewTarget("work", resolve, testHome)
	notices := target.Notices
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
	if !found["remoteforward"] || !found["controlmaster"] {
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
	target, err := sshclient.NewTarget("host", resolve, testHome)
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
	target, err := sshclient.NewTarget("host", resolve, testHome)
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

	target, err := sshclient.NewTarget("work", resolve, testHome)
	notices := target.Notices
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

// 転送は Target に載り、notice にはならない。**実装されたからである。**
func TestForwardsAreCarriedRatherThanNoticed(t *testing.T) {
	resolve := resolverFor(map[string]map[string][]string{
		"work": {
			"hostname":       {"10.0.0.9"},
			"localforward":   {"8080 10.0.0.5:80"},
			"dynamicforward": {"1080"},
			"forwardagent":   {"yes"},
		},
	})
	target, err := sshclient.NewTarget("work", resolve, testHome)
	notices := target.Notices
	if err != nil {
		t.Fatal(err)
	}
	if len(notices) != 0 {
		t.Fatalf("an implemented feature still produced notices: %#v", notices)
	}
	if !target.AgentForward {
		t.Error("ForwardAgent yes did not reach the target")
	}
	if len(target.Forwards) != 2 {
		t.Fatalf("forwards = %#v", target.Forwards)
	}
	// 並びは固定である。接続のたびに一覧が並び替わってはならない。
	if target.Forwards[0].Kind != "dynamic" || target.Forwards[1].Kind != "local" {
		t.Fatalf("forwards = %#v, want a stable order", target.Forwards)
	}
	if target.Forwards[1].To != "10.0.0.5:80" || target.Forwards[1].ListenPort != "8080" {
		t.Errorf("forward = %#v", target.Forwards[1])
	}
}

// **bind するのはループバックだけである。** それ以外が書かれていたら束ねて
// notice を出す——転送の設定ひとつで繋がらなくなる方が困る。
func TestANonLoopbackBindIsFoldedOntoLoopback(t *testing.T) {
	resolve := resolverFor(map[string]map[string][]string{
		"work": {"hostname": {"10.0.0.9"}, "localforward": {"0.0.0.0:8080 10.0.0.5:80"}},
	})
	target, err := sshclient.NewTarget("work", resolve, testHome)
	notices := target.Notices
	if err != nil {
		t.Fatal(err)
	}
	if len(target.Forwards) != 1 || target.Forwards[0].Address() != "127.0.0.1:8080" {
		t.Fatalf("forwards = %#v", target.Forwards)
	}
	if len(notices) != 1 || !strings.Contains(notices[0].Detail, "127.0.0.1") {
		t.Fatalf("notices = %#v, want the fold to be said out loud", notices)
	}
}

// 読めない値は notice を出して飛ばす。書式ひとつで接続できなくなる理由が無い。
func TestAnUnreadableForwardIsSkippedRatherThanFatal(t *testing.T) {
	resolve := resolverFor(map[string]map[string][]string{
		"work": {
			"hostname":     {"10.0.0.9"},
			"localforward": {"nonsense", "8080 10.0.0.5:80"},
		},
	})
	target, err := sshclient.NewTarget("work", resolve, testHome)
	notices := target.Notices
	if err != nil {
		t.Fatalf("an unreadable forward refused the connection: %v", err)
	}
	if len(target.Forwards) != 1 {
		t.Fatalf("forwards = %#v, want the readable one", target.Forwards)
	}
	if len(notices) != 1 {
		t.Fatalf("notices = %#v", notices)
	}
}

// HostKeyAlgorithms は、交渉で名乗る順を人が決める指定である。
//
// **書かれていればそれが順序である。** OpenSSH はこの指定があるとき
// known_hosts による並べ替えを行わない。先頭の一文字（+ - ^）も OpenSSH が
// 決めている形であり、それぞれ既定へ足す・既定から外す・既定の先頭へ移す。
// 書かれていなければ空である。**ここが埋まっていると known_hosts が黙らされる。**
// `ssh -G` は指定が無くても既定の一覧を出力するが、この解決器が既定を入れるのは
// hostname と user と port の三つだけであり、それに頼っている。
func TestHostKeyAlgorithmsStaysEmptyWhenNothingWasWritten(t *testing.T) {
	resolve := resolverFor(map[string]map[string][]string{"host": {"hostname": {"10.0.0.9"}}})
	target, err := sshclient.NewTarget("host", resolve, testHome)
	if err != nil {
		t.Fatal(err)
	}
	if len(target.HostKeyAlgorithms) != 0 {
		t.Errorf("algorithms = %#v, want known_hosts to decide", target.HostKeyAlgorithms)
	}
}

func TestHostKeyAlgorithmsFollowsWhatWasWritten(t *testing.T) {
	algorithmsFor := func(t *testing.T, written string) []string {
		t.Helper()
		resolve := resolverFor(map[string]map[string][]string{
			"host": {"hostname": {"10.0.0.9"}, "hostkeyalgorithms": {written}},
		})
		target, err := sshclient.NewTarget("host", resolve, testHome)
		if err != nil {
			t.Fatal(err)
		}
		return target.HostKeyAlgorithms
	}

	// 素の並びは既定を置き換える。
	if got := algorithmsFor(t, "ssh-ed25519,rsa-sha2-512"); len(got) != 2 ||
		got[0] != "ssh-ed25519" || got[1] != "rsa-sha2-512" {
		t.Errorf("plain list = %#v", got)
	}

	// + は既定の後ろへ足す。
	appended := algorithmsFor(t, "+ssh-dss")
	if len(appended) < 2 || appended[len(appended)-1] != "ssh-dss" {
		t.Errorf("+ssh-dss = %#v", appended)
	}
	if appended[0] != "ssh-ed25519" {
		t.Errorf("+ssh-dss changed the head of the default: %#v", appended)
	}

	// - は既定から外す。**外す側だけがパターンを受け取る。**
	kept := algorithmsFor(t, "-ecdsa-sha2-*")
	if len(kept) == 0 {
		t.Fatal("-ecdsa-sha2-* removed everything")
	}
	for _, algorithm := range kept {
		if strings.HasPrefix(algorithm, "ecdsa") {
			t.Errorf("-ecdsa-sha2-* left %q behind: %#v", algorithm, kept)
		}
	}

	// ^ は既定の先頭へ移す。二度は現れない。
	moved := algorithmsFor(t, "^rsa-sha2-256")
	if len(moved) == 0 || moved[0] != "rsa-sha2-256" {
		t.Fatalf("^rsa-sha2-256 = %#v", moved)
	}
	seen := 0
	for _, algorithm := range moved {
		if algorithm == "rsa-sha2-256" {
			seen++
		}
	}
	if seen != 1 {
		t.Errorf("^rsa-sha2-256 appears %d times: %#v", seen, moved)
	}
}
