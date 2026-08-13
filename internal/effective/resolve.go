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
	RefusalMatchExec      = "match_exec"
	RefusalMatchUnknown   = "match_unsupported"
	RefusalMatchFinal     = "match_final"
	RefusalCanonicalize   = "canonicalize_hostname"
	RefusalUnknownToken   = "unknown_token"
	RefusalUnresolvedPath = "unresolved_include"
)

// Refusal は、この設定について答えを出さない理由ひとつ。
type Refusal struct {
	Code   string
	Path   string
	Line   int
	Detail string
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

// expandsTokens は、解決の時点でトークンを展開するキーワードを列挙する。
//
// **HostName だけである。実機の ssh -G で確かめた。** IdentityFile と
// CertificateFile のトークンは、-G の出力では展開されないまま出てくる——
// OpenSSH がそれらを展開するのは接続する瞬間であり、設定を読み終えた時点では
// ない。ここで展開すると、設定について報告する値が ssh の報告とずれる。
//
// 接続に使うときの展開はプロセス内 SSH クライアント（B2）の仕事である。
// ExpandTokens はそのために置いてある。
var expandsTokens = map[string]bool{"hostname": true}

// Resolve は、この alias に接続したときに実際に使われる値を返す。
//
// **何も実行しない。** Match exec を含む設定は、値ではなく理由を返す。部分的な
// 答えを黙って返さないのは、接続に使う値がひとつでも確定しないなら、その alias は
// 解決できていないからである。
func Resolve(graph *config.Graph, alias string, facts LocalFacts) (Values, []Refusal) {
	values := Values{Entries: map[string][]string{}}
	if graph == nil {
		return values, nil
	}

	var refusals []Refusal
	claimed := map[string]bool{}
	applies := true

	set := func(keyword, value string) {
		lowered := strings.ToLower(keyword)
		if claimed[lowered] && !cumulativeKeywords[lowered] {
			return
		}
		if _, seen := values.Entries[lowered]; !seen {
			values.Keywords = append(values.Keywords, lowered)
		}
		values.Entries[lowered] = append(values.Entries[lowered], value)
		claimed[lowered] = true
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

	enterBlock := func(filePath string, _ *config.File, block config.Block) {
		switch block.Kind {
		case config.BlockGlobal:
			applies = true
		case config.BlockHost:
			_, applies = blockApplies(block, alias)
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
		set(line.Keyword, argumentText(line))
	}

	walkLoadOrder(graph, graph.Root, map[string]bool{}, enterBlock, directive)

	for _, diagnostic := range graph.Diagnostics {
		if diagnostic.Severity == config.SeverityInfo {
			continue
		}
		refusals = append(refusals, Refusal{
			Code: RefusalUnresolvedPath, Path: diagnostic.Path,
			Line: diagnostic.Line, Detail: diagnostic.Code,
		})
	}

	if len(refusals) > 0 {
		// 部分的な答えを返さない。ひとつでも確定しないなら解決できていない。
		return Values{Entries: map[string][]string{}}, refusals
	}

	applyDefaults(&values, alias, facts)
	if refusal, ok := expandAll(&values, alias, facts); !ok {
		return Values{Entries: map[string][]string{}}, []Refusal{refusal}
	}
	return values, nil
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
		for index, entry := range values.Entries[keyword] {
			expanded, err := ExpandTokens(entry, facts, target)
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
		if keyword == "hostname" || !expandsTokens[keyword] {
			continue
		}
		if refusal, ok := expand(keyword, target); !ok {
			return refusal, false
		}
	}
	return Refusal{}, true
}

func valueOr(values Values, keyword, fallback string) string {
	if found := values.First(keyword); found != "" {
		return found
	}
	return fallback
}
