package app

import (
	"crypto/rand"
	"testing"

	"sshc/internal/handoff"
	"sshc/internal/secret"
)

// **時計で閉じるかどうかは、engine を起こした側が決める。**
//
// デスクトップの engine はアプリの子であり、アプリを終えれば道連れに死ぬ——
// vault はメモリの中だけなので、そこで消える。蓋を閉じたノートは、開ければ
// OS がログインパスワードを訊く。そこへ 8 時間の時計を重ねても、増える安全は
// わずかで、確実に増えるのは再入力の回数である。
//
// 画面の無い機械は違う。`sshc headless` は systemd の下で何週間も走り、蓋も
// 画面ロックも無い。**そこでは、これが唯一の歯止めである。**
//
// ここが確かめるのは、その判断が実際に届いていることである——秘密を持つ側で
// 正しく振る舞っても、渡し忘れていれば意味が無い。
func TestTheVaultClosesOnAClockOnlyWhereThereIsNoScreen(t *testing.T) {
	for _, expected := range []struct {
		owner handoff.Owner
		idle  string
	}{
		{handoff.OwnerDesktop, "開いたまま"},
		{handoff.OwnerHeadless, "8 時間で閉じる"},
	} {
		t.Run(string(expected.owner), func(t *testing.T) {
			services, err := newEngineServices(Dependencies{
				Home:   t.TempDir(),
				Owner:  expected.owner,
				Random: rand.Reader,
			})
			if err != nil {
				t.Fatal(err)
			}

			idle := services.passwords.IdleTimeout()
			want := secret.IdleTimeout
			if expected.owner == handoff.OwnerDesktop {
				want = secret.StayOpen
			}
			if idle != want {
				t.Errorf("%s の engine は %s であるべきだが、idle=%v だった",
					expected.owner, expected.idle, idle)
			}
		})
	}
}
