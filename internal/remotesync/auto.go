package remotesync

import (
	"context"
	"errors"
	"sync"
	"time"
)

// AutoInterval は、巡回の間隔。
//
// **短くしても、速くはならない。** 一巡でやるのは、ローカルを数えることと
// HEAD を 1 本投げることだけであり、それより細かく見ても、人が押した瞬間より
// 早く気づけるわけではない。1 台につき 1 日 1440 回の HEAD は、この用途の
// バケットではまず無視できる。
const AutoInterval = time.Minute

// AutoPhase は、巡回がいまどこに居るかである。
type AutoPhase string

const (
	// AutoIdle は、直近の巡回が何事もなく終わったことである。
	AutoIdle AutoPhase = "idle"
	// AutoRunning は、いま巡回していることである。
	AutoRunning AutoPhase = "running"
	// AutoBlocked は、**人の判断を待っている**ことである。衝突があったか、
	// 適用すれば何かが消えるかのどちらかで、どちらも自動では踏み越えない。
	AutoBlocked AutoPhase = "blocked"
	// AutoFailed は、巡回が届かなかったことである。次の巡回でまた試す。
	AutoFailed AutoPhase = "failed"
)

// AutoView は、画面が自動同期について言えることのすべてである。
type AutoView struct {
	// Enabled は、この設置で自動同期が入っているか。
	Enabled bool      `json:"enabled"`
	Phase   AutoPhase `json:"phase"`
	// Detail は、blocked と failed のときの理由である。人へ見せる文ではなく、
	// 画面が自分の言葉に訳すための符牒である。
	Detail string `json:"detail,omitempty"`
	// At は、直近の巡回が終わった時刻。
	At string `json:"at,omitempty"`
}

// Auto は、人が押さなくても同期を進める巡回である。
//
// **押さなくても進むことと、黙って壊すことは違う。** 衝突と、何かが消える適用は
// 自動では行わない——そこは人が見るべき分岐であり、待っていることを画面が言う。
type Auto struct {
	service *Service
	// Key は、封をする鍵を取り出す。取れないなら（保管庫が閉じている、鍵がまだ
	// 無い）、この巡回は何もしない。**保管庫が開いていることが、同期してよいこと
	// の唯一の条件である。**
	Key func() (string, bool)
	// Enabled は、この設置で自動同期が入っているかを答える。設定は保管庫の中に
	// あるので、閉じていれば false が返る——それで正しい。
	Enabled func() bool

	interval time.Duration
	now      func() string

	// cycleMu は、一巡が重ならないようにする。時計が来たときと、人が「今すぐ」を
	// 押したときが同時に起きうる。
	cycleMu sync.Mutex

	mu   sync.Mutex
	view AutoView
}

// NewAuto は、巡回を組み立てる。走り出すのは Run が呼ばれてからである。
func NewAuto(service *Service, interval time.Duration, now func() string) *Auto {
	if interval <= 0 {
		interval = AutoInterval
	}
	return &Auto{service: service, interval: interval, now: now}
}

// View は、画面へ渡す形の現在地。
func (a *Auto) View() AutoView {
	a.mu.Lock()
	defer a.mu.Unlock()
	view := a.view
	view.Enabled = a.enabled()
	return view
}

func (a *Auto) enabled() bool { return a.Enabled != nil && a.Enabled() }

// Run は、ctx が終わるまで巡回し続ける。
func (a *Auto) Run(ctx context.Context) {
	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.Once(ctx)
		}
	}
}

// Once は一巡し、その結果を返す。時計が来たときも、人が「今すぐ」を押したときも
// 通るのはここである。**押した人には、その一巡で何が起きたかが返る**——次の
// 巡回まで待ってから状態を読み直す画面は、押したことが効いたのかを言えない。
//
// **先に受け取り、それから送る。** 逆にすると、向こうが進んでいた場合の push は
// 必ず条件で弾かれ、毎回 1 往復を捨てることになる。
func (a *Auto) Once(ctx context.Context) AutoView {
	a.cycleMu.Lock()
	defer a.cycleMu.Unlock()
	if !a.enabled() {
		return a.View()
	}
	key, ok := a.keyFor()
	if !ok {
		return a.View()
	}
	a.enter(AutoRunning, "")

	if phase, detail, done := a.receive(ctx, key); done {
		a.enter(phase, detail)
		return a.View()
	}
	phase, detail := a.send(ctx, key)
	a.enter(phase, detail)
	return a.View()
}

func (a *Auto) keyFor() (string, bool) {
	if a.Key == nil {
		return "", false
	}
	key, ok := a.Key()
	return key, ok && key != ""
}

// receive は、向こうが進んでいれば取り込む。done が true なら、そこで一巡を
// 終える——人の判断を待つ状態で push を続けても、押し込むものが増えるだけである。
func (a *Auto) receive(ctx context.Context, key string) (AutoPhase, string, bool) {
	if a.service.Direction() == DirectionPush {
		return AutoIdle, "", false
	}
	// 動いていないものは取りに行かない。HEAD は ETag だけを返す。
	moved, err := a.service.RemoteMoved(ctx)
	if err != nil {
		return AutoFailed, failureDetail(err), true
	}
	if !moved {
		return AutoIdle, "", false
	}
	// **巡回は寄せ先を選ばない。** どちらを残すかは人が決める分岐である。
	result, err := a.service.Pull(ctx, key, ResolveNone)
	switch {
	case errors.Is(err, ErrNothingToApply), errors.Is(err, ErrNoSnapshot):
		return AutoIdle, "", false
	case err != nil:
		return AutoFailed, failureDetail(err), true
	}
	if len(result.Conflicts) > 0 {
		return AutoBlocked, "conflicts", true
	}
	// **消すものがあるなら、自動では適用しない。** 置き換えは控えが残り History
	// から戻せるが、消えたファイルは画面から消える。ここは人が見る分岐である。
	if len(result.Request.Removals) > 0 {
		return AutoBlocked, "removals", true
	}
	if len(result.Request.Changes) == 0 {
		return AutoIdle, "", false
	}
	if err := a.service.Apply(result); err != nil {
		if errors.Is(err, ErrApplyRefused) {
			return AutoIdle, "", false
		}
		return AutoFailed, failureDetail(err), true
	}
	return AutoIdle, "", false
}

// send は、こちらが進んでいれば押し出す。
func (a *Auto) send(ctx context.Context, key string) (AutoPhase, string) {
	if a.service.Direction() == DirectionPull {
		return AutoIdle, ""
	}
	changed, err := a.service.Diverged()
	if err != nil {
		return AutoFailed, failureDetail(err)
	}
	if !changed {
		return AutoIdle, ""
	}
	if _, err := a.service.Push(ctx, key); err != nil {
		switch {
		case errors.Is(err, ErrPushRefused):
			return AutoIdle, ""
		case errors.Is(err, ErrRemoteMoved):
			// 次の巡回が先に受け取る。失敗ではなく、順番の問題である。
			return AutoIdle, ""
		}
		return AutoFailed, failureDetail(err)
	}
	return AutoIdle, ""
}

func (a *Auto) enter(phase AutoPhase, detail string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.view.Phase = phase
	a.view.Detail = detail
	if phase != AutoRunning {
		a.view.At = a.now()
	}
}

// failureDetail は、画面が訳せる符牒だけを残す。**エラー文そのものを残さない**
// ——入口の URL や bucket の名前が混ざりうる。
func failureDetail(err error) string {
	switch {
	case errors.Is(err, ErrNotConfigured):
		return "not_configured"
	case errors.Is(err, ErrRemoteMoved):
		return "remote_moved"
	case errors.Is(err, ErrConflicts):
		return "conflicts"
	}
	return "unreachable"
}
