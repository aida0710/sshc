package effective

import (
	"errors"
	"strings"

	"sshc/internal/config"
	"sshc/internal/sshmatch"
)

// ErrMatchExec は、この解決器が Match exec を評価しないことを報告する。
//
// 解決器は何も実行しない。それが、評価に確認ダイアログを要らなくしている
// 理由である。exec の結果に依存する設定は、値を推測するのではなく解決できないと
// 返す。~/.ssh/config は基準のまま残るので、そのユーザーは端末から ssh で繋げる。
var ErrMatchExec = errors.New("Match exec is not evaluated")

// ErrMatchUnsupported は、まだ評価できない Match 属性を報告する。
//
// localnetwork はローカルのアドレスを列挙しないと判定できない。判定できない
// ものを真として扱えば、書いていないブロックを適用することになる。
var ErrMatchUnsupported = errors.New("that Match criterion is not evaluated")

// MatchContext は、Match の条件を判定するために要る事実。
//
// OpenSSH は接続の途中でこれらを決める。ここではすべて解決の入力として渡される
// ので、判定にプロセスを起動する必要がない。
type MatchContext struct {
	// Alias は、いま解決している名前。canonical 化しないので OriginalAlias と
	// 同じ値になるが、OpenSSH の用語に合わせて二つ持つ。
	Alias string
	// OriginalAlias は、利用者が打った名前。
	OriginalAlias string
	// HostName は、ここまでに解決した HostName。OpenSSH の `Match host` は alias
	// ではなくこれと比較する（`HostName` の %h は alias で展開済み）。まだ
	// HostName が決まっていなければ alias と同じ値になる。
	HostName string
	// User は、ここまでに解決したリモートのアカウント名。
	User string
	// LocalUser は、このマシンのアカウント名。
	LocalUser string
	// Tags は、ここまでに解決した Tag の値。
	Tags []string
	// Final は、OpenSSH の二周目にあたるかどうか。
	Final bool
	// Canonical は、ホスト名の canonical 化を経たかどうか。この解決器は
	// canonical 化しないので常に false である。
	Canonical bool
}

// hostForMatch は `Match host` が比べる名前を返す。HostName が空の文脈は
// まだ HostName を解決していないので、OpenSSH と同じく alias を使う。
func (context MatchContext) hostForMatch() string {
	if context.HostName != "" {
		return context.HostName
	}
	return context.Alias
}

// MatchApplies は、Match ブロックがこの文脈に適用されるかを報告する。
//
// すべての条件が真のときだけ適用される。OpenSSH は Match 行の属性を AND で
// 繋ぐので、ひとつでも外れれば、そのブロックは何も寄与しない。
func MatchApplies(criteria []config.Criterion, context MatchContext) (bool, error) {
	if len(criteria) == 0 {
		return false, nil
	}
	for _, criterion := range criteria {
		matched, err := criterionApplies(criterion, context)
		if err != nil {
			return false, err
		}
		if criterion.Negated {
			matched = !matched
		}
		if !matched {
			return false, nil
		}
	}
	return true, nil
}

func criterionApplies(criterion config.Criterion, context MatchContext) (bool, error) {
	patterns := strings.Split(criterion.Argument, ",")
	switch strings.ToLower(criterion.Keyword) {
	case "all":
		return true, nil
	case "canonical":
		return context.Canonical, nil
	case "final":
		return context.Final, nil
	case "host":
		// host と originalhost は OpenSSH の match_hostname で比べる。値もパターンも
		// ASCII を小文字にしてから比べるので、HostName に大文字があっても一致する。
		return sshmatch.PatternList(patterns, context.hostForMatch(), sshmatch.IgnoreCase), nil
	case "originalhost":
		return sshmatch.PatternList(patterns, context.OriginalAlias, sshmatch.IgnoreCase), nil
	case "user":
		return sshmatch.PatternList(patterns, context.User, sshmatch.CaseSensitive), nil
	case "localuser":
		return sshmatch.PatternList(patterns, context.LocalUser, sshmatch.CaseSensitive), nil
	case "tagged":
		for _, tag := range context.Tags {
			if sshmatch.PatternList(patterns, tag, sshmatch.CaseSensitive) {
				return true, nil
			}
		}
		return false, nil
	case "exec":
		return false, ErrMatchExec
	default:
		// localnetwork と、OpenSSH が後から足すであろう属性。真として扱えば
		// 書いていないブロックを適用することになるので、判定できないと言う。
		return false, ErrMatchUnsupported
	}
}
