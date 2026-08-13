package effective

import (
	"errors"
	"strings"

	"sshc/internal/config"
)

// 解決を諦める理由。
//
// Complexity とは別のものである。あちらは「説明はできるが単純ではない」という
// 印で、値は出る。こちらは**値を出さない**理由である。
const (
	RefusalMatchExec    = "match_exec"
	RefusalMatchUnknown = "match_unsupported"
	RefusalMatchFinal   = "match_final"
	RefusalCanonicalize = "canonicalize_hostname"
	RefusalUnknownToken = "unknown_token"
)

// Refusal は、この設定について答えを出さない理由ひとつ。
type Refusal struct {
	Code   string
	Path   string
	Line   int
	Detail string
}

// Accepted は、解決が採用したディレクティブひとつと、それが書かれている場所。
//
// 値と出所を同じ走査から出すのは、別々に歩くとまた「同じ問いに答えるものが 2 つ」
// になるからである。画面が出所を示せるのは、決定そのものがここから来ているときだけ
// 意味を持つ。
type Accepted struct {
	Keyword   string
	Values    []string
	Path      string
	Line      int
	Condition string
}

// Resolution は、ひとつの alias についての答えである。
//
// Refusals が空でないとき、Values と Accepted は空である。部分的な答えを黙って
// 返さない——接続に使う値がひとつでも確定しないなら、その alias は解決できていない。
type Resolution struct {
	Values   Values
	Accepted []Accepted
	Refusals []Refusal
	// Notes は、答えは確定しているが読み手が知っておくべきこと。
	//
	// 権威になる前は、これらは「だから ssh -G に委ねる」という意味の
	// complexity だった。いまは答えが出るので意味が変わる——同じ alias を
	// 二つのブロックが主張していても、勝つのはどちらかが決まっている。
	// それでも書いた本人には見えていないので、印は残す。
	Notes []Complexity
}

// defaultPort は、Port が書かれていないときの値。
const defaultPort = "22"

// IdentityFile に既定値は持たない。
//
// **これは実測に基づく判断である。** 一度は OpenSSH の既定の並びを写したが、
// 差分テストが macOS と Linux で違う答えを返した——Linux 側のビルドは
// ~/.ssh/id_xmss を含んでいた。版とビルドオプションで変わる表であり、
// 「OpenSSH の既定値表を丸ごと持たない」という判断がここにも当てはまる。
//
// 書かれていなければ、この解決器は IdentityFile を答えない。接続に使う鍵を
// 選ぶのはプロセス内 SSH クライアント（B2）であり、そちらは OpenSSH の探索順
// ではなく、利用者が選んだ鍵と鍵の一覧を使う。

// expandsTokens は、解決の時点でトークンを展開するキーワードと、そこで
// 許されるトークンを対応づける。
//
// **HostName だけである。実機の ssh -G で確かめた。** IdentityFile と
// CertificateFile のトークンは、-G の出力では展開されないまま出てくる——
// OpenSSH がそれらを展開するのは接続する瞬間であり、設定を読み終えた時点では
// ない。ここで展開すると、設定について報告する値が ssh の報告とずれる。
//
// **HostName が受け付けるのは %% と %h だけである。これも実機で確かめた。**
// `HostName %r.example.com` は "unknown key %r" で落ちる。全トークンを
// 展開すると、本物なら起動しない設定に、こちらだけが答えを出すことになる。
//
// 接続に使うときの展開はプロセス内 SSH クライアント（B2）の仕事である。
// ExpandTokens はそのために置いてある。
var expandsTokens = map[string]string{"hostname": "h"}

// Resolve は、この alias に接続したときに実際に使われる値を返す。
//
// **何も実行しない。** Match exec を含む設定は、値ではなく理由を返す。部分的な
// 答えを黙って返さないのは、接続に使う値がひとつでも確定しないなら、その alias は
// 解決できていないからである。
func Resolve(graph *config.Graph, alias string, facts LocalFacts) Resolution {
	values := Values{Entries: map[string][]string{}}
	if graph == nil {
		return Resolution{Values: values}
	}

	var refusals []Refusal
	var accepted []Accepted
	var notes []Complexity
	matchedHostBlocks := 0
	claimed := map[string]bool{}
	applies := true

	set := func(keyword, value string) bool {
		// 引数の無いディレクティブは値を主張しない。`User` とだけ書かれた行を
		// 通すと、user が空文字のまま接続に使われる——それは alias と同じ扱いを
		// 受けるべき欠落であって、確定した空の値ではない。本物の ssh は設定
		// 全体を撥ねるが、こちらは書かれていないものとして既定値を埋める。
		// 行そのものは config の診断が別に報告する。
		lowered := strings.ToLower(keyword)
		if value == "" {
			return false
		}
		if claimed[lowered] && !cumulativeKeywords[lowered] {
			return false
		}
		if _, seen := values.Entries[lowered]; !seen {
			values.Keywords = append(values.Keywords, lowered)
		}
		values.Entries[lowered] = append(values.Entries[lowered], value)
		claimed[lowered] = true
		return true
	}

	// Match の判定は、そこまでに解決した値を見る。OpenSSH も同じ順で決めるので、
	// 走査しながら組み立てる。
	context := func() MatchContext {
		user := valueOr(values, "user", facts.User)
		return MatchContext{
			Alias: alias, OriginalAlias: alias, User: user, LocalUser: facts.User,
			Tags: values.Entries["tag"],
		}
	}

	condition := ""
	enterBlock := func(filePath string, file *config.File, block config.Block) {
		condition = file.Condition(block)
		switch block.Kind {
		case config.BlockGlobal:
			applies = true
		case config.BlockHost:
			var kind string
			kind, applies = blockApplies(block, alias)
			if !applies {
				return
			}
			// 数えるのは alias を名指ししているブロックだけである。たまたま
			// 一致した catch-all は「二つのブロックがこの名前を主張している」
			// ではない。それはワイルドカードで一致したという別の話である。
			if declaresExactly(block.Patterns, alias) {
				matchedHostBlocks++
				if matchedHostBlocks > 1 {
					notes = append(notes, Complexity{
						Code: ComplexityDuplicateAlias, Path: filePath, Line: block.Header + 1,
						Condition: condition, Detail: "more than one Host block claims this alias",
					})
				}
			}
			if kind == SourceWildcard {
				notes = append(notes, Complexity{
					Code: ComplexityWildcardPattern, Path: filePath, Line: block.Header + 1,
					Condition: condition, Detail: "this block matched through a wildcard pattern",
				})
			}
			for _, pattern := range block.Patterns {
				if !pattern.Negated {
					continue
				}
				notes = append(notes, Complexity{
					Code: ComplexityNegatedPattern, Path: filePath, Line: block.Header + 1,
					Condition: condition, Detail: "this block excludes hosts through " + pattern.Raw,
				})
				break
			}
		case config.BlockMatch:
			applies = false
			for _, criterion := range block.Criteria {
				if strings.EqualFold(criterion.Keyword, "final") {
					refusals = append(refusals, Refusal{
						Code: RefusalMatchFinal, Path: filePath, Line: block.Header + 1,
						Detail: "Match final needs a second pass this resolver does not make",
					})
					return
				}
			}
			matched, err := MatchApplies(block.Criteria, context())
			switch {
			case errors.Is(err, ErrMatchExec):
				refusals = append(refusals, Refusal{
					Code: RefusalMatchExec, Path: filePath, Line: block.Header + 1,
					Detail: "this resolver runs nothing, so Match exec cannot be evaluated",
				})
			case err != nil:
				refusals = append(refusals, Refusal{
					Code: RefusalMatchUnknown, Path: filePath, Line: block.Header + 1,
					Detail: err.Error(),
				})
			default:
				applies = matched
			}
		}
	}

	directive := func(filePath string, index int, line config.Line) {
		if !applies {
			return
		}
		if config.EqualKeyword(line.Keyword, "CanonicalizeHostname") &&
			!strings.EqualFold(argumentText(line), "no") {
			refusals = append(refusals, Refusal{
				Code: RefusalCanonicalize, Path: filePath, Line: index + 1,
				Detail: "canonicalisation re-reads the configuration, which this resolver does not do",
			})
			return
		}
		if set(line.Keyword, argumentText(line)) {
			accepted = append(accepted, Accepted{
				Keyword: line.Keyword, Values: line.Values(),
				Path: filePath, Line: index + 1, Condition: condition,
			})
		}
	}

	walkLoadOrder(graph, graph.Root, map[string]bool{}, enterBlock, directive)

	// 読めない Include は拒否ではなく印である。**読めた範囲で解決する。**
	//
	// 拒否にすると、まだ作られていないディレクトリを Include が指している間、
	// その alias について何も答えられなくなる。グループを作る保存はまさにその
	// 状態を通るので、保存前後の比較が空になっていた。
	for _, diagnostic := range graph.Diagnostics {
		if diagnostic.Severity == config.SeverityInfo {
			continue
		}
		notes = append(notes, Complexity{
			Code: ComplexityUnresolvedInclude, Path: diagnostic.Path,
			Line: diagnostic.Line, Detail: diagnostic.Code,
		})
	}

	if len(refusals) > 0 {
		// 部分的な答えを返さない。ひとつでも確定しないなら解決できていない。
		return Resolution{Values: Values{Entries: map[string][]string{}}, Refusals: refusals}
	}

	applyDefaults(&values, alias, facts)
	if refusal, ok := expandAll(&values, alias, facts); !ok {
		return Resolution{
			Values:   Values{Entries: map[string][]string{}},
			Refusals: []Refusal{refusal},
		}
	}
	return Resolution{Values: values, Accepted: accepted, Notes: notes}
}

// applyDefaults は、この解決器が既定値を持つ 5 つだけを埋める。
//
// 書かれていない他のキーワードには触れない。OpenSSH の既定値表を丸ごと持つのは、
// 版ごとに変わるものを追い続ける保守であり、利用者に何も返さない。
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
	fill("port", defaultPort)
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
		Port:       valueOr(*values, "port", defaultPort),
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

// declaresExactly は、Host 行がパターンによる一致ではなくこの alias を名指しして
// いるかを報告する。catch-all は全 alias に一致し、何も宣言しない。
func declaresExactly(patterns []config.Pattern, alias string) bool {
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
