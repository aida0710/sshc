package effective_test

import (
	"errors"
	"testing"

	"sshc/internal/config"
	"sshc/internal/effective"
)

func matchContext() effective.MatchContext {
	return effective.MatchContext{
		Alias: "db", OriginalAlias: "db", User: "ops", LocalUser: "aida",
		Tags: []string{"production"},
	}
}

func TestMatchAppliesEvaluatesEveryCriterionWithoutRunningAnything(t *testing.T) {
	for _, test := range []struct {
		name     string
		criteria []config.Criterion
		want     bool
	}{
		{"all", []config.Criterion{{Keyword: "all"}}, true},
		{"host matches", []config.Criterion{{Keyword: "host", Argument: "db"}}, true},
		{"host misses", []config.Criterion{{Keyword: "host", Argument: "web"}}, false},
		{"host list", []config.Criterion{{Keyword: "host", Argument: "web,db"}}, true},
		{"host wildcard", []config.Criterion{{Keyword: "host", Argument: "d*"}}, true},
		{"originalhost", []config.Criterion{{Keyword: "originalhost", Argument: "db"}}, true},
		{"user", []config.Criterion{{Keyword: "user", Argument: "ops"}}, true},
		{"localuser", []config.Criterion{{Keyword: "localuser", Argument: "aida"}}, true},
		{"tagged", []config.Criterion{{Keyword: "tagged", Argument: "production"}}, true},
		{"tagged misses", []config.Criterion{{Keyword: "tagged", Argument: "staging"}}, false},
		// 属性そのものへの否定。
		{"negated host", []config.Criterion{{Keyword: "host", Argument: "db", Negated: true}}, false},
		{"negated miss", []config.Criterion{{Keyword: "host", Argument: "web", Negated: true}}, true},
		// パターン側の否定。当たった時点でその属性が偽になる。
		{"pattern negation", []config.Criterion{{Keyword: "host", Argument: "!db,*"}}, false},
		// すべての条件が真のときだけ適用される。
		{"user and host", []config.Criterion{
			{Keyword: "host", Argument: "db"}, {Keyword: "user", Argument: "ops"},
		}, true},
		{"one criterion fails", []config.Criterion{
			{Keyword: "host", Argument: "db"}, {Keyword: "user", Argument: "root"},
		}, false},
		// この解決器は canonical 化しないので canonical は常に偽、final も
		// 呼び出し側が真にしない限り偽である。
		{"canonical", []config.Criterion{{Keyword: "canonical"}}, false},
		{"final", []config.Criterion{{Keyword: "final"}}, false},
		// 条件のない Match 行は何も適用しない。
		{"no criteria", nil, false},
	} {
		got, err := effective.MatchApplies(test.criteria, matchContext())
		if err != nil {
			t.Errorf("%s: MatchApplies = %v", test.name, err)
			continue
		}
		if got != test.want {
			t.Errorf("%s: MatchApplies = %v, want %v", test.name, got, test.want)
		}
	}
}

// 解決器は何も実行しない。exec の結果に依存する設定は、値を推測するのではなく
// 解決できないと答える。
func TestMatchAppliesRefusesToRunAnything(t *testing.T) {
	_, err := effective.MatchApplies([]config.Criterion{{Keyword: "exec", Argument: "true"}}, matchContext())
	if !errors.Is(err, effective.ErrMatchExec) {
		t.Errorf("Match exec = %v, want ErrMatchExec", err)
	}

	// ただし、先に外れる条件があれば exec には届かない。OpenSSH は Match 行の
	// 属性を左から評価し、外れた時点で打ち切るので、そのブロックの exec は走らない
	// ——走らないものを理由に解決を拒むと、繋げる設定を繋げないと言うことになる。
	//
	// これは OpenSSH の評価順に依存している。差分テストがそこを見張る。
	applies, err := effective.MatchApplies([]config.Criterion{
		{Keyword: "host", Argument: "nothing"}, {Keyword: "exec", Argument: "true"},
	}, matchContext())
	if err != nil || applies {
		t.Errorf("a block that cannot apply = %v, %v; want false and no refusal", applies, err)
	}
}

// 答えられない属性を真として扱えば、書いていないブロックを適用することになる。
func TestMatchAppliesSaysWhenItCannotAnswer(t *testing.T) {
	_, err := effective.MatchApplies(
		[]config.Criterion{{Keyword: "localnetwork", Argument: "192.0.2.0/24"}}, matchContext())
	if !errors.Is(err, effective.ErrMatchUnsupported) {
		t.Errorf("Match localnetwork = %v, want ErrMatchUnsupported", err)
	}
}
