package effective

import (
	"errors"
	"strings"

	"sshc/internal/config"
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
	switch strings.ToLower(criterion.Keyword) {
	case "all":
		return true, nil
	case "canonical":
		return context.Canonical, nil
	case "final":
		return context.Final, nil
	case "host":
		return matchesAny(criterion.Argument, context.Alias), nil
	case "originalhost":
		return matchesAny(criterion.Argument, context.OriginalAlias), nil
	case "user":
		return matchesAny(criterion.Argument, context.User), nil
	case "localuser":
		return matchesAny(criterion.Argument, context.LocalUser), nil
	case "tagged":
		for _, tag := range context.Tags {
			if matchesAny(criterion.Argument, tag) {
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

// matchesAny は、カンマ区切りのパターン列のどれかに一致するかを報告する。
//
// 否定は Match 行の属性そのものに付く（`!host`）ほか、パターン側にも書ける
// （`Match host !web,*`）。後者は OpenSSH と同じく、否定に当たった時点で
// その属性全体が偽になる。
func matchesAny(argument, value string) bool {
	matched := false
	for _, pattern := range strings.Split(argument, ",") {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if negated := strings.HasPrefix(pattern, "!"); negated {
			if MatchPattern(strings.TrimPrefix(pattern, "!"), value) {
				return false
			}
			continue
		}
		if MatchPattern(pattern, value) {
			matched = true
		}
	}
	return matched
}
