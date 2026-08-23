package effective

import (
	"errors"
	"strings"

	"sshc/internal/config"
)

const (
	// DefaultJumpPort は、ポート指定のないホップに OpenSSH が使うポート。
	DefaultJumpPort = "22"
	// MaxJumpDepth は、入れ子になった ProxyJump をどこまでたどるかを制限する。
	MaxJumpDepth = 8
	// MaxRouteStages は、展開された経路ひとつが含みうるホップ数を制限する。
	//
	// MaxJumpDepth だけでは走査は制限されない。カンマ区切りリストの
	// 各ホップが自身のリストを持ちうるので、段数は和ではなくリスト長の
	// 積として増える。この上限で止められた経路は
	// ComplexityJumpDepth を報告する。
	MaxRouteStages = 256
	// jumpDisabled は、ProxyJump を無効にするリテラル。
	jumpDisabled = "none"
)

// ErrInvalidJump は、このエンジンが解釈を拒む ProxyJump の値を報告する。
var ErrInvalidJump = errors.New("ProxyJump value is not a valid destination list")

// Hop は ProxyJump リストの行き先ひとつ。UserExplicit と PortExplicit は、その値が
// リスト自体から来たのかを記録する。リストに書かれた値は、そのホップ自身の設定に
// 勝つからである。
type Hop struct {
	Raw          string
	User         string
	Host         string
	Port         string
	UserExplicit bool
	PortExplicit bool
}

// Chain は解析済みの ProxyJump の値。
type Chain struct {
	Raw      string
	Disabled bool
	Hops     []Hop
}

// jumpTokens は、OpenSSH が ProxyJump に許すトークンである。
//
// ssh_config(5) の TOKENS が「ProxyCommand and ProxyJump accept the tokens
// %%, %h, %n, %p, and %r」と言っている。いずれも最終的な行き先の値を指す
// ——手前のホップではない。
const jumpTokens = "hnpr"

// ExpandChainTokens は、ProxyJump の値のトークンを展開する。
//
// **解決器はここを生のまま返す。** `ssh -G` がそうするからであり、この製品が
// 設定について報告する値は ssh の報告と一致していなければならない。OpenSSH が
// これを展開するのは繋ぐ瞬間なので、展開するのも繋ぐ側である。
//
// 展開しないと、`ProxyJump %r@gateway` は「%r という名前の利用者」として
// ゲートウェイに認証を試みることになる。publickey は当然通らず、残る方式も
// 無いので、握手はそこで終わる——実際そうなっていた。
//
// 許した 5 つ以外は拒む。展開できないまま残せば、その文字列がユーザー名や
// ホスト名としてそのまま使われる。
func ExpandChainTokens(raw string, target TokenTarget) (string, error) {
	if !usesOnlyTokens(raw, jumpTokens) {
		return "", ErrUnknownToken
	}
	// ProxyJump はローカルの事実（%u、%d、%i、%l）を受け取らない。空の
	// LocalFacts を渡すのは、上の検査がそれらを既に拒しているからである。
	return ExpandTokens(raw, LocalFacts{}, target)
}

// ParseChain は、単一またはカンマ区切りの ProxyJump 値を読む。
func ParseChain(raw string) (Chain, error) {
	chain := Chain{Raw: raw}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return chain, nil
	}
	if strings.EqualFold(trimmed, jumpDisabled) {
		chain.Disabled = true
		return chain, nil
	}

	for _, element := range strings.Split(trimmed, ",") {
		element = strings.TrimSpace(element)
		if element == "" {
			return Chain{}, ErrInvalidJump
		}
		hop := Hop{Raw: element, Port: DefaultJumpPort}
		destination := element
		if at := strings.LastIndex(destination, "@"); at >= 0 {
			hop.User = destination[:at]
			hop.UserExplicit = true
			destination = destination[at+1:]
			if hop.User == "" || destination == "" {
				return Chain{}, ErrInvalidJump
			}
		}
		switch {
		case strings.HasPrefix(destination, "["):
			closing := strings.Index(destination, "]")
			if closing < 0 {
				return Chain{}, ErrInvalidJump
			}
			hop.Host = destination[1:closing]
			if remainder := destination[closing+1:]; remainder != "" {
				if !strings.HasPrefix(remainder, ":") {
					return Chain{}, ErrInvalidJump
				}
				hop.Port = remainder[1:]
				hop.PortExplicit = true
			}
		default:
			if colon := strings.LastIndex(destination, ":"); colon >= 0 && !strings.Contains(destination[:colon], ":") {
				hop.Host = destination[:colon]
				hop.Port = destination[colon+1:]
				hop.PortExplicit = true
			} else {
				hop.Host = destination
			}
		}
		if hop.Host == "" || hop.Port == "" {
			return Chain{}, ErrInvalidJump
		}
		chain.Hops = append(chain.Hops, hop)
	}
	return chain, nil
}

// Stage は経路のホップひとつ。API と UI が再帰型を必要としないよう平坦化してある。
// Depth は、対象自身の ProxyJump リストについては 0。自身の ProxyJump を持つ踏み台
// ホストは、次の深さに段を寄与する。
type Stage struct {
	Order    int
	Depth    int
	Parent   string
	Hop      Hop
	Hostname string
	User     string
	Port     string
	Complex  bool
}

// ExpandRoute は、alias とその中のすべての踏み台ホストの ProxyJump 連鎖を展開する。
// これにより、最初のホップだけでなく経路の全体を表示できる。
//
// **値は Resolve から取る。** かつてここは Project を使っていた。あれは Match
// ブロックを一切適用しない——条件が接続中の状態に依るからで、「どの行が書いたか」
// を並べる用途ではそれで正しい。**だがここが答えているのは出所ではなく、踏み台へ
// 実際に繋ぐ宛先である。**
//
// 症状は静かではなかったが、正しくもなかった: Project は Match の存在を
// complexity として記録するので段は「単純ではない」と印されたものの、HostName と
// User と Port そのものは間違ったまま画面に出た。そして **ProxyJump 自体が Match
// の下に書かれていれば、経路は丸ごと空になった** ——画面は「踏み台を通らない」と
// 言い、実際には通る。
func ExpandRoute(graph *config.Graph, alias string, facts LocalFacts) ([]Stage, []Complexity) {
	walk := routeWalk{
		graph:     graph,
		facts:     facts,
		ancestors: map[string]bool{strings.ToLower(alias): true},
	}
	return walk.expand(alias, 0)
}

// routeWalk は、ExpandRoute 一回分の状態を運ぶ。
//
// ancestors は、開始 alias から、いま展開しているホップまでの経路上にある alias を
// ちょうど保持し、走査が戻るときに巻き戻される。循環とは、自分自身の経路上に再び
// 現れるホップのことだ。永久に再帰しうるのはそれだけだからである。別の枝を通って
// 再び到達される踏み台ホストは普通の形であり、そこでも
// 展開される。
type routeWalk struct {
	graph     *config.Graph
	facts     LocalFacts
	ancestors map[string]bool
	order     int
}

func (w *routeWalk) expand(alias string, depth int) ([]Stage, []Complexity) {
	resolution := Resolve(w.graph, alias, w.facts)
	// **解決を諦めたことを、経路が無いことにしない。** Match exec を含む設定は
	// 値を出さない。黙って空の経路を返せば、画面は「踏み台を通らない」と言う。
	if len(resolution.Refusals) > 0 {
		return nil, []Complexity{{
			Code:   ComplexityJumpUnresolved,
			Path:   resolution.Refusals[0].Path,
			Line:   resolution.Refusals[0].Line,
			Detail: resolution.Refusals[0].Detail,
		}}
	}
	raw := resolution.Values.First("proxyjump")
	if raw == "" {
		return nil, nil
	}
	where := whereAccepted(resolution, "proxyjump")
	chain, err := ParseChain(raw)
	if err != nil {
		return nil, []Complexity{{
			Code:   ComplexityJumpInvalid,
			Path:   where.Path,
			Line:   where.Line,
			Detail: raw,
		}}
	}
	if chain.Disabled {
		return nil, nil
	}
	if depth >= MaxJumpDepth {
		return nil, []Complexity{{
			Code:   ComplexityJumpDepth,
			Path:   where.Path,
			Line:   where.Line,
			Detail: "the jump route is deeper than this engine follows",
		}}
	}

	var stages []Stage
	var complexities []Complexity
	for _, hop := range chain.Hops {
		if w.order >= MaxRouteStages {
			complexities = append(complexities, Complexity{
				Code:   ComplexityJumpDepth,
				Path:   where.Path,
				Line:   where.Line,
				Detail: "the jump route has more hops than this engine follows",
			})
			break
		}

		hopResolution := Resolve(w.graph, hop.Host, w.facts)
		w.order++
		stage := Stage{
			Order:  w.order,
			Depth:  depth,
			Parent: alias,
			Hop:    hop,
			// **リストに書かれた値は、そのホップ自身の設定に勝つ。** OpenSSH が
			// そうする。書かれていなければ、解決した値が答えである。
			Hostname: hop.Host,
			User:     hop.User,
			Port:     hop.Port,
			// **印は残す。** 値は確定していても、同じ alias を二つのブロックが
			// 主張しているようなことは、書いた本人には見えていない。
			Complex: len(hopResolution.Notes) > 0 || len(hopResolution.Refusals) > 0,
		}
		if len(hopResolution.Refusals) == 0 {
			stage.Hostname = valueOr(hopResolution.Values, "hostname", hop.Host)
			if !hop.UserExplicit {
				stage.User = valueOr(hopResolution.Values, "user", hop.User)
			}
			if !hop.PortExplicit {
				stage.Port = valueOr(hopResolution.Values, "port", hop.Port)
			}
		}
		stages = append(stages, stage)

		lowered := strings.ToLower(hop.Host)
		if w.ancestors[lowered] {
			complexities = append(complexities, Complexity{
				Code:   ComplexityJumpCycle,
				Path:   where.Path,
				Line:   where.Line,
				Detail: hop.Host + " already appears earlier in this route",
			})
			continue
		}
		w.ancestors[lowered] = true
		nestedStages, nestedComplexities := w.expand(hop.Host, depth+1)
		delete(w.ancestors, lowered)
		stages = append(stages, nestedStages...)
		complexities = append(complexities, nestedComplexities...)
	}
	return stages, complexities
}

// whereAccepted は、そのキーワードを採用した行の場所を返す。
//
// **値と場所を別々に歩かない。** Resolve は採用した行をそのまま持っているので、
// 印に添える綴りと行番号はそこから取る。
func whereAccepted(resolution Resolution, keyword string) Accepted {
	for _, accepted := range resolution.Accepted {
		if strings.EqualFold(accepted.Keyword, keyword) {
			return accepted
		}
	}
	return Accepted{}
}
