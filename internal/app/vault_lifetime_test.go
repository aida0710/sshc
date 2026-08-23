package app

import (
	"crypto/rand"
	"testing"

	"sshc/internal/handoff"
	"sshc/internal/secret"
)

// **時計は、engine を誰が起こしたかで変わらない。**
//
// 以前はデスクトップだけ外していた。根拠は「engine はアプリの子であり、アプリを
// 終えれば道連れに死ぬ」だったが、**その親が居なくなった。** コマンドが engine を
// 起こしてブラウザを開く形では、窓を閉じても engine は生き続ける——忘れられた
// 端末の中で何日も開いたままになりうるなら、それは画面の無い機械と同じ状況で
// あり、そこでは時計が唯一の歯止めである。
//
// ここが確かめるのは、その判断が実際に届いていることである——秘密を持つ側で
// 正しく振る舞っても、渡し忘れていれば意味が無い。
func TestTheVaultClosesOnTheSameClockWhoeverStartedTheEngine(t *testing.T) {
	for _, owner := range []handoff.Owner{handoff.OwnerEngine, handoff.OwnerEngine} {
		t.Run(string(owner), func(t *testing.T) {
			services, err := newEngineServices(Dependencies{
				Home:   t.TempDir(),
				Owner:  owner,
				Random: rand.Reader,
			})
			if err != nil {
				t.Fatal(err)
			}

			if idle := services.passwords.IdleTimeout(); idle != secret.IdleTimeout {
				t.Errorf("%s の engine は %v で閉じるべきだが、idle=%v だった",
					owner, secret.IdleTimeout, idle)
			}
		})
	}
}

// **12 時間である。** 朝に開いた人は夜まで訊かれず、翌朝には訊かれる。
func TestTheClockIsTwelveHours(t *testing.T) {
	if hours := secret.IdleTimeout.Hours(); hours != 12 {
		t.Fatalf("IdleTimeout = %v 時間, want 12", hours)
	}
}
