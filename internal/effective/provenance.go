package effective

import (
	"strings"

	"sshc/internal/config"
)

// 出所をどれだけ確信をもって説明できるか。
const (
	SourceExact    = "exact"
	SourceWildcard = "wildcard"
	SourceGlobal   = "global"
)

// cumulativeKeywords は、最初の値だけを残すのではなく OpenSSH が積み上げる
// ディレクティブである。他のキーワードはすべて先勝ちに従う。Resolve と Project が
// この表を共有し、累積の判定を一か所に保つ。
//
// SetEnv はここに無い。実機の ssh -G で確かめた結果である。二行書くと
// 最初の行しか出力されない。複数の変数を渡すには `SetEnv ONE=1 TWO=2` と
// 一行に並べる。SendEnv は ssh_config(5) が「複数の SendEnv に分けてよい」と
// 明記しているので残す。
var cumulativeKeywords = map[string]bool{
	"identityfile": true, "certificatefile": true, "localforward": true,
	"remoteforward": true, "dynamicforward": true, "sendenv": true,
}

// 射影を、ひとつの整った継承の連鎖として示せない理由。
const (
	ComplexityWildcardPattern   = "wildcard_pattern"
	ComplexityNegatedPattern    = "negated_pattern"
	ComplexityMatchBlock        = "match_block"
	ComplexityDuplicateAlias    = "duplicate_alias"
	ComplexityProxyIgnored      = "proxy_ignored"
	ComplexityUnresolvedInclude = "unresolved_include"
	ComplexityJumpInvalid       = "jump_invalid"
	ComplexityJumpCycle         = "jump_cycle"
	ComplexityJumpDepth         = "jump_depth_exceeded"
	// ComplexityJumpUnresolved は、経路を辿る前に解決そのものを諦めたことを言う。
	//
	// 空の経路と区別する。暗黙に空を返せば、画面は「踏み台を通らない」と
	// 言う。Match exec を含む設定では、通るかどうかがまさに分からない。
	ComplexityJumpUnresolved = "jump_unresolved"
)

// Source は、値の出どころひとつ。Winner は OpenSSH が採用する値を示す。最初に
// 読まれた値が勝つからだ。他のものも列挙するのは、何が影に隠れているかを読み手が
// 見られるようにするためである。
type Source struct {
	Keyword   string
	Value     string
	Path      string
	Line      int
	Condition string
	Kind      string
	Winner    bool
}

// Complexity は、エンジンが自身の射影を全体の真実として提示することを拒む理由を
// 記録する。UI はこれらを複雑な外部ルールとして示し、権威ある値については
// `ssh -G` に委ねる。
type Complexity struct {
	Code      string
	Path      string
	Line      int
	Condition string
	Detail    string
}

// Projection は、ひとつの alias に対するエンジン自身の設定の読み。
type Projection struct {
	Alias        string
	Sources      []Source
	Complexities []Complexity
}

// Project は設定を読み込み順に走査し、各キーワードを、それを最初に設定した
// ブロックへ帰属させる。これは OpenSSH がしていることである。
//
// 値を決めるのは Resolve であって、これではない。ここが返すのは「どの行が
// この値を書いたのか」と「なぜ言い切れないのか」であり、internal/diagnostics の
// 表示にだけ使う。接続値や認証情報の判定には、Match ブロックを含む Resolve の結果を
// 直接使う。
//
// 読み込み順と Include の扱いは Resolve と同じ walkLoadOrder に従う。生成された
// グループの領域は、Include をユーザーの catch-all の上に置いてグループの値を
// 既定に勝たせている。ファイル順で帰属させると、その場合の出所を逆に報告する。
//
// Match ブロックが値を寄与することは決してない。その条件は、接続中にしか存在しない
// 状態に依存するからである。代わりに、到達する Match ブロックは complexity として
// 記録される。それは、この射影があとの Host ブロックへ帰属させた値を、影に隠す
// こともできるからだ。
func Project(graph *config.Graph, alias string) Projection {
	projection := Projection{Alias: alias}
	if graph == nil {
		return projection
	}
	claimed := make(map[string]bool)
	hostNotes := hostBlockNotes{alias: alias}

	enterBlock := func(header blockHeader) (string, bool) {
		if header.block.Kind == config.BlockMatch {
			if !header.neverMatch {
				projection.Complexities = append(projection.Complexities, Complexity{
					Code: ComplexityMatchBlock, Path: header.path, Line: header.number(), Condition: header.condition,
					Detail: "Match criteria are evaluated while connecting, so this block may override values shown here",
				})
			}
			return "", false
		}
		kind, applies := hostBlockApplies(header.block, alias)
		if applies && !header.neverMatch {
			projection.Complexities = append(projection.Complexities, hostNotes.observe(header, kind)...)
		}
		return kind, applies
	}

	directive := func(found directiveLine) {
		keyword := strings.ToLower(found.line.Keyword)
		projection.Sources = append(projection.Sources, Source{
			Keyword:   found.line.Keyword,
			Value:     strings.Join(directiveValues(found.line), " "),
			Path:      found.path,
			Line:      found.number,
			Condition: found.state.condition,
			Kind:      found.state.kind,
			// 積み上がるキーワードは、二行目以降も採用される。OpenSSH が
			// そうするので、一律の先勝ちで印を付けると誤りになる。
			Winner: !claimed[keyword] || cumulativeKeywords[keyword],
		})
		claimed[keyword] = true
	}

	walkLoadOrder(graph, loadOrderVisitor{enterBlock: enterBlock, directive: directive})
	projection.Complexities = append(projection.Complexities, unresolvedIncludeNotes(graph.Diagnostics)...)
	return projection
}
