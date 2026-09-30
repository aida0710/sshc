package effective

import (
	"errors"
	"net/netip"
	"strings"

	"sshc/internal/config"
	"sshc/internal/sshmatch"
)

// 解決を諦める理由。
//
// Complexity とは別のものである。あちらは「説明はできるが単純ではない」という
// 印で、値は出る。こちらは値を出さない理由である。
const (
	RefusalMatchExec    = "match_exec"
	RefusalMatchUnknown = "match_unsupported"
	RefusalMatchFinal   = "match_final"
	RefusalCanonicalize = "canonicalize_hostname"
	RefusalUnknownToken = "unknown_token"
)

// Refusal は、この設定について結果を出さない理由ひとつ。
type Refusal struct {
	Code   string
	Path   string
	Line   int
	Detail string
}

// Accepted は、解決が採用したディレクティブひとつと、それが書かれている場所。
//
// 値と出所を同じ走査から生成し、両者の不一致を防ぐ。
type Accepted struct {
	Keyword   string
	Values    []string
	Path      string
	Line      int
	Condition string
}

// Resolution は、ひとつの alias についての結果である。
//
// Refusals が空でないとき、Values と Accepted は空である。部分的な結果を暗黙に
// 返さない。接続に使う値がひとつでも確定しないなら、その alias は解決できていない。
type Resolution struct {
	Values   Values
	Accepted []Accepted
	Refusals []Refusal
	// Notes は、結果は確定しているが読み手が知っておくべきこと。
	//
	// 同じ alias を二つのブロックが主張していても、勝つ方は決まっている。それでも
	// 書いた本人には見えていないので、印を残す。
	Notes []Complexity
}

// IdentityFile に既定値は持たない。
//
// OpenSSH の既定の並びはバージョンとビルドオプションで変わる。差分テストでは、Linux の
// ビルドだけが ~/.ssh/id_xmss を含んでいた。「OpenSSH の既定値表を丸ごと持たない」
// という判断がここにも当てはまる。
//
// 書かれていなければ、この解決器は IdentityFile を応答しない。接続に使う鍵を
// 選ぶのは internal/sshclient であり、そちらは OpenSSH の探索順ではなく、利用者が
// 選んだ鍵と鍵の一覧を使う。

// expandsTokens は、解決の時点でトークンを展開するキーワードと、そこで
// 許されるトークンを対応づける。
//
// HostName だけである。実機の ssh -G で確かめた。IdentityFile と
// CertificateFile のトークンは、-G の出力では展開されないまま出てくる。
// OpenSSH がそれらを展開するのは接続する瞬間であり、設定を読み終えた時点では
// ない。ここで展開すると、設定について報告する値が ssh の報告とずれる。
//
// HostName が受け付けるのは %% と %h だけである。これも実機で確かめた。
// `HostName %r.example.com` は "unknown key %r" で落ちる。全トークンを
// 展開すると、本物なら起動しない設定に、こちらだけが結果を出すことになる。
//
// 接続に使うときの展開は internal/sshclient の仕事である。ExpandTokens は
// そのために置いてある。
var expandsTokens = map[string]string{"hostname": "h"}

// Resolve は、この alias に接続したときに実際に使われる値を返す。
//
// 何も実行しない。Match exec を含む設定は、値ではなく理由を返す。部分的な
// 結果を暗黙に返さないのは、接続に使う値がひとつでも確定しないなら、その alias は
// 解決できていないからである。
func Resolve(graph *config.Graph, alias string, facts LocalFacts) Resolution {
	if graph == nil {
		return Resolution{Values: Values{Entries: map[string][]string{}}}
	}
	walk := newResolveWalk(alias, facts)
	walkLoadOrder(graph, loadOrderVisitor{enterBlock: walk.enterBlock, directive: walk.directive})
	walk.notes = append(walk.notes, unresolvedIncludeNotes(graph.Diagnostics)...)

	if len(walk.refusals) > 0 {
		// 部分的な結果を返さない。ひとつでも確定しないなら解決できていない。
		return Resolution{Values: Values{Entries: map[string][]string{}}, Refusals: walk.refusals}
	}

	values := walk.values
	applyDefaults(&values, alias, facts)
	if refusal, ok := expandAll(&values, alias, facts); !ok {
		return Resolution{
			Values:   Values{Entries: map[string][]string{}},
			Refusals: []Refusal{refusal},
		}
	}
	lowerHostName(&values)
	return Resolution{Values: values, Accepted: walk.accepted, Notes: walk.notes}
}

// lowerHostName は、確定した HostName を OpenSSH と同じく小文字にする。
//
// ssh.c は HostName の %h を展開したあと、アドレスのリテラルでなければ lowercase()
// する。`HostName 2001:DB8::1` は `ssh -G` でもそのまま出る。小文字にするのは ASCII
// の英字だけで、全角の文字などは変えない。
func lowerHostName(values *Values) {
	for index, hostName := range values.Entries["hostname"] {
		if _, err := netip.ParseAddr(hostName); err == nil {
			continue
		}
		values.Entries["hostname"][index] = sshmatch.LowerASCII(hostName)
	}
}

// resolveWalk は、Resolve が読み込み順の走査のあいだに積み上げるものである。
type resolveWalk struct {
	alias     string
	facts     LocalFacts
	values    Values
	claimed   map[string]bool
	accepted  []Accepted
	refusals  []Refusal
	notes     []Complexity
	hostNotes hostBlockNotes
}

func newResolveWalk(alias string, facts LocalFacts) *resolveWalk {
	return &resolveWalk{
		alias: alias, facts: facts,
		values:    Values{Entries: map[string][]string{}},
		claimed:   map[string]bool{},
		hostNotes: hostBlockNotes{alias: alias},
	}
}

func (w *resolveWalk) enterBlock(header blockHeader) (string, bool) {
	if header.block.Kind == config.BlockMatch {
		return "", w.enterMatch(header)
	}
	kind, applies := hostBlockApplies(header.block, w.alias)
	if applies && !header.neverMatch {
		w.notes = append(w.notes, w.hostNotes.observe(header, kind)...)
	}
	return kind, applies
}

// enterMatch は、Match ブロックがこの alias に効くかを決める。決められなければ
// refusals に理由を積み、効かないと答える。
func (w *resolveWalk) enterMatch(header blockHeader) bool {
	for _, criterion := range header.block.Criteria {
		// 一致しない Include の中の Match final でも、OpenSSH は二周目を始める。
		if strings.EqualFold(criterion.Keyword, "final") {
			w.refuse(header, RefusalMatchFinal, "Match final needs a second pass this resolver does not make")
			return false
		}
	}
	// 一致しない Include の中の Match は、評価の結果にかかわらず効かない。値は
	// 決まるので、exec を評価できないことは拒否の理由にならない。
	if header.neverMatch {
		return false
	}
	matched, err := MatchApplies(header.block.Criteria, w.matchContext())
	switch {
	case errors.Is(err, ErrMatchExec):
		w.refuse(header, RefusalMatchExec, "this resolver runs nothing, so Match exec cannot be evaluated")
	case err != nil:
		w.refuse(header, RefusalMatchUnknown, err.Error())
	}
	return err == nil && matched
}

func (w *resolveWalk) refuse(header blockHeader, code, detail string) {
	w.refusals = append(w.refusals, Refusal{Code: code, Path: header.path, Line: header.number(), Detail: detail})
}

// matchContext は、Match の判定が見る「そこまでに解決した値」である。OpenSSH も
// 同じ順で決めるので、走査しながら組み立てる。
func (w *resolveWalk) matchContext() MatchContext {
	return MatchContext{
		Alias: w.alias, OriginalAlias: w.alias, HostName: matchHostName(w.values, w.alias),
		User: valueOr(w.values, "user", w.facts.User), LocalUser: w.facts.User, Tags: w.values.Entries["tag"],
	}
}

func (w *resolveWalk) directive(found directiveLine) {
	line := found.line
	entries := directiveValues(line)
	if config.EqualKeyword(line.Keyword, "CanonicalizeHostname") &&
		!strings.EqualFold(strings.Join(entries, " "), "no") {
		w.refusals = append(w.refusals, Refusal{
			Code: RefusalCanonicalize, Path: found.path, Line: found.number,
			Detail: "canonicalisation re-reads the configuration, which this resolver does not do",
		})
		return
	}
	if ignored, reason := proxyDirectiveIgnored(line.Keyword, w.values); ignored {
		w.notes = append(w.notes, Complexity{
			Code: ComplexityProxyIgnored, Path: found.path, Line: found.number,
			Condition: found.state.condition, Detail: reason,
		})
		return
	}
	if w.set(line.Keyword, entries) {
		w.accepted = append(w.accepted, Accepted{
			Keyword: line.Keyword, Values: entries,
			Path: found.path, Line: found.number, Condition: found.state.condition,
		})
	}
}

// set は、ディレクティブ一行の値を採用するかを決め、採用したら values に積む。
//
// 引数の無いディレクティブは値を主張しない。`User` とだけ書かれた行を通すと、
// user が空文字のまま接続に使われる。それは書かれていないのと同じ扱いを受ける
// べき欠落であって、確定した空の値ではない。本物の ssh は設定全体を撥ねるが、
// こちらは既定値を埋める。行そのものは config の診断が別に報告する。
func (w *resolveWalk) set(keyword string, entries []string) bool {
	lowered := strings.ToLower(keyword)
	if len(entries) == 0 {
		return false
	}
	if w.claimed[lowered] && !cumulativeKeywords[lowered] {
		return false
	}
	if _, seen := w.values.Entries[lowered]; !seen {
		w.values.Keywords = append(w.values.Keywords, lowered)
	}
	w.values.Entries[lowered] = append(w.values.Entries[lowered], entries...)
	w.claimed[lowered] = true
	return true
}

// proxyDirectiveIgnored は、ProxyCommand と ProxyJump のうち後から来た方を OpenSSH が
// 黙って捨てる規則を再現する。`ssh -G` で確かめた: ProxyJump（none 以外）の後の
// ProxyCommand は無視、ProxyCommand（none を含む）の後の ProxyJump は無視。
// `ProxyJump none` の後の ProxyCommand だけはバージョンで違う。readconf.c の
// CVE-2026-35386 対応（parse_jump の書き直し）より前の OpenSSH は "none" を
// 「ProxyJump 設定済み」と数えて後の ProxyCommand を捨て、対応後（Ubuntu の
// 10.2p1 にパッチを当てたもの、上流はそれ以降）は有効にする。ここは新しい方に合わせる。
// 捨てた行は Notes に残し、書いた本人が「効いていない」ことを見られるようにする。
func proxyDirectiveIgnored(keyword string, values Values) (bool, string) {
	switch strings.ToLower(keyword) {
	case "proxycommand":
		if jump := values.First("proxyjump"); jump != "" && !strings.EqualFold(jump, "none") {
			return true, "ProxyCommand is ignored because ProxyJump was already set"
		}
	case "proxyjump":
		if values.First("proxycommand") != "" {
			return true, "ProxyJump is ignored because ProxyCommand was already set"
		}
	}
	return false, ""
}

// applyDefaults は、この解決器が既定値を持つ 3 つ（hostname、user、port）だけを埋める。
//
// 書かれていない他のキーワードには触れない。OpenSSH の既定値表を丸ごと持つのは、
// バージョンごとに変わるものを追い続ける保守であり、利用者に何も返さない。
func applyDefaults(values *Values, alias string, facts LocalFacts) {
	fill := func(keyword string, candidates ...string) {
		if len(values.Entries[keyword]) > 0 {
			return
		}
		for _, candidate := range candidates {
			if candidate == "" {
				continue
			}
			values.Keywords = append(values.Keywords, keyword)
			values.Entries[keyword] = append(values.Entries[keyword], candidate)
		}
	}
	fill("hostname", alias)
	fill("user", facts.User)
	fill("port", DefaultPort)
}

// matchHostName は、`Match host` が比較する「ここまでに解決した HostName」を返す。
// OpenSSH は match_cfg_line で options->hostname の %h を alias で展開して比べる。
// HostName が未確定なら alias そのものである。展開できない値はそのまま比べる。
// 走査の後で expandAll が同じ値を拒むので、ここで refusal を重ねない。
func matchHostName(values Values, alias string) string {
	hostName := valueOr(values, "hostname", alias)
	expanded, err := ExpandTokens(hostName, LocalFacts{}, TokenTarget{Alias: alias, HostName: alias})
	if err != nil {
		return hostName
	}
	return expanded
}

// expandAll は、トークンを受け取るキーワードの値を展開する。
//
// 走査のあとに一度だけ行う。%h と %p と %r は解決の結果なので、走査しながら
// 展開すると、まだ決まっていない値で潰すことになる。
//
// HostName の中の %h だけは元の alias を指す。OpenSSH がそう決めているからで、
// `HostName %h.example.com` は自分自身を参照しない。
func expandAll(values *Values, alias string, facts LocalFacts) (Refusal, bool) {
	expand := func(keyword string, target TokenTarget) (Refusal, bool) {
		allowed := expandsTokens[keyword]
		for index, entry := range values.Entries[keyword] {
			expanded, err := ExpandTokens(entry, facts, target)
			if err == nil && !usesOnlyTokens(entry, allowed) {
				err = ErrUnknownToken
			}
			if err != nil {
				return Refusal{
					Code:   RefusalUnknownToken,
					Detail: keyword + " uses a token this resolver does not expand: " + entry,
				}, false
			}
			values.Entries[keyword][index] = expanded
		}
		return Refusal{}, true
	}

	// HostName を先に展開する。他のキーワードの %h は解決後の HostName を指すので、
	// 先に確定させないと、展開前の文字列で潰すことになる。HostName 自身の中の
	// %h は元の alias であり、自分自身を参照しない。
	if refusal, ok := expand("hostname", TokenTarget{Alias: alias, HostName: alias}); !ok {
		return refusal, false
	}

	target := TokenTarget{
		Alias:      alias,
		HostName:   valueOr(*values, "hostname", alias),
		Port:       valueOr(*values, "port", DefaultPort),
		RemoteUser: valueOr(*values, "user", facts.User),
	}
	for keyword := range values.Entries {
		if _, expands := expandsTokens[keyword]; keyword == "hostname" || !expands {
			continue
		}
		if refusal, ok := expand(keyword, target); !ok {
			return refusal, false
		}
	}
	return Refusal{}, true
}

// usesOnlyTokens は、値に現れる %X が allowed にあるものと %% だけかを報告する。
//
// ExpandTokens が知っているトークンの集合と、そのキーワードが受け付ける集合は
// 別である。前者は接続の瞬間に使えるすべてで、後者は OpenSSH が設定を読む時点で
// そのディレクティブに許すものである。
func usesOnlyTokens(value, allowed string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] != '%' {
			continue
		}
		index++
		if index >= len(value) {
			return false
		}
		if value[index] != '%' && strings.IndexByte(allowed, value[index]) < 0 {
			return false
		}
	}
	return true
}

func valueOr(values Values, keyword, fallback string) string {
	if found := values.First(keyword); found != "" {
		return found
	}
	return fallback
}

// DeclaresExactly は、Host ブロックのパターンが alias をそのまま名指ししているかを
// 返す。ワイルドカードや否定でたまたま一致するブロックは「この alias を主張する
// ブロック」には数えない。
func DeclaresExactly(patterns []config.Pattern, alias string) bool {
	for _, pattern := range patterns {
		if pattern.Negated || pattern.Wildcard {
			continue
		}
		if pattern.Value == alias {
			return true
		}
	}
	return false
}
