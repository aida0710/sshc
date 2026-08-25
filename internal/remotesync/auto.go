package remotesync

import (
	"context"
	"errors"
	"sync"
	"time"
)

// AutoInterval は自動同期の確認間隔。
const AutoInterval = time.Minute

// AutoPhase は自動同期の現在状態。
type AutoPhase string

const (
	// AutoIdle は直近の巡回が完了した状態。
	AutoIdle AutoPhase = "idle"
	// AutoRunning は巡回中の状態。
	AutoRunning AutoPhase = "running"
	// AutoBlocked は競合または削除についてユーザーの判断を待つ状態。
	AutoBlocked AutoPhase = "blocked"
	// AutoFailed は巡回が失敗し、次回に再試行する状態。
	AutoFailed AutoPhase = "failed"
)

// AutoView は、画面が自動同期について言えることのすべてである。
type AutoView struct {
	// Enabled は、この設置で自動同期が入っているか。
	Enabled bool      `json:"enabled"`
	Phase   AutoPhase `json:"phase"`
	// Detail は、blocked と failed のときの理由である。ユーザーへ見せる文ではなく、
	// 画面が自分の言葉に訳すための符牒である。
	Detail string `json:"detail,omitempty"`
	// At は、直近の巡回が終わった時刻。
	At string `json:"at,omitempty"`
}

// Auto は定期的に同期する。競合と削除は自動適用せず AutoBlocked として報告する。
type Auto struct {
	service *Service
	// Key は同期鍵を返す。vault がロック中または未設定なら false を返す。
	Key func() (string, bool)
	// Enabled は、この設置で自動同期が入っているかを返す。設定は保管庫の中に
	// あるので、閉じていれば false が返る。それで正しい。
	Enabled func() bool
	// Unattended は、自動処理による参照を vault の利用時刻に数えないようにする。
	Unattended func(run func())

	interval time.Duration
	now      func() string

	// cycleMu は、一巡が重ならないようにする。時計が来たときと、ユーザーが「今すぐ」を
	// 押したときが同時に起きうる。
	cycleMu sync.Mutex

	mu   sync.Mutex
	view AutoView
	// blockedETag is the remote generation which needs a human decision. A
	// ticker still performs HEAD, but does not download or derive a key again
	// until that generation changes or a manual Apply advances local state.
	blockedETag   string
	blockedDetail string
}

// NewAuto は、巡回を組み立てる。走り出すのは Run が呼ばれてからである。
func NewAuto(service *Service, interval time.Duration, now func() string) *Auto {
	if interval <= 0 {
		interval = AutoInterval
	}
	return &Auto{
		service:  service,
		interval: interval,
		now:      now,
		view:     AutoView{Phase: AutoIdle},
	}
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

// Once は同期を一巡し、その結果を返す。リモート更新の取込後にローカル変更を送信する。
func (a *Auto) Once(ctx context.Context) AutoView {
	a.cycleMu.Lock()
	defer a.cycleMu.Unlock()
	if a.Unattended != nil {
		var view AutoView
		a.Unattended(func() { view = a.run(ctx) })
		return view
	}
	return a.run(ctx)
}

func (a *Auto) run(ctx context.Context) AutoView {
	if !a.enabled() {
		return a.View()
	}
	key, ok := a.keyFor()
	if !ok {
		return a.View()
	}
	a.service.operationMu.Lock()
	defer a.service.operationMu.Unlock()
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

// receive はリモート更新を取り込む。done はこの巡回を終了すべきことを示す。
func (a *Auto) receive(ctx context.Context, key string) (AutoPhase, string, bool) {
	if a.service.Direction() == DirectionPush {
		return AutoIdle, "", false
	}
	// 動いていないものは取りに行かない。HEAD は ETag だけを返す。
	moved, remoteETag, err := a.service.remoteGeneration(ctx)
	if err != nil {
		return AutoFailed, failureDetail(err), true
	}
	if !moved {
		a.clearBlocked()
		return AutoIdle, "", false
	}
	if detail, ok := a.blocked(remoteETag); ok {
		return AutoBlocked, detail, true
	}
	// 自動同期では競合の解決先を選ばない。
	result, err := a.service.pull(ctx, key, ResolveNone, "")
	switch {
	case errors.Is(err, ErrNoSnapshot):
		return AutoIdle, "", false
	case err != nil && !errors.Is(err, ErrNothingToApply):
		return AutoFailed, failureDetail(err), true
	}
	if len(result.Conflicts) > 0 {
		a.rememberBlocked(result.ETag, "conflicts")
		return AutoBlocked, "conflicts", true
	}
	// 削除はユーザーの確認が必要なため自動適用しない。
	if len(result.Request.Removals) > 0 {
		a.rememberBlocked(result.ETag, "removals")
		return AutoBlocked, "removals", true
	}
	// 差分がなくてもApplyはremote世代を記録する。これを省くと同じsnapshotを
	// 毎分取得し続け、次のpushも古いETagで拒否される。
	if err := a.service.apply(result); err != nil {
		if errors.Is(err, ErrApplyRefused) {
			return AutoIdle, "", false
		}
		return AutoFailed, failureDetail(err), true
	}
	return AutoIdle, "", false
}

// send はローカルに変更があれば push する。
func (a *Auto) send(ctx context.Context, key string) (AutoPhase, string) {
	if a.service.Direction() == DirectionPull {
		return AutoIdle, ""
	}
	changed, err := a.service.diverged()
	if err != nil {
		return AutoFailed, failureDetail(err)
	}
	if !changed {
		return AutoIdle, ""
	}
	if _, err := a.service.push(ctx, key, ""); err != nil {
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

func (a *Auto) rememberBlocked(etag, detail string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.blockedETag = etag
	a.blockedDetail = detail
}

func (a *Auto) blocked(etag string) (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.blockedDetail, etag != "" && a.blockedETag == etag
}

func (a *Auto) clearBlocked() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.blockedETag = ""
	a.blockedDetail = ""
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

// failureDetail は、機密情報を含みうるエラー文を返さず、画面用の安定した code に変換する。
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
