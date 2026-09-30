package remotesync

import (
	"context"
	"errors"
	"sync"
	"time"
)

// AutoInterval はリモート更新を受信確認する間隔。
const AutoInterval = time.Minute

// AutoPushDelay は、最後のローカル変更から自動送信まで待つ時間。
// ひとつの画面操作が複数の永続化処理を伴っても、1つのsnapshotへまとめる。
const AutoPushDelay = 5 * time.Second

// autoBackoffMaxは、一時的な失敗が続いた同じ世代を再び試すまでの待ちの上限。
// 同期先の障害が続くあいだは要求を数分に1回へ抑え、直ったあとは遅くとも5分で同期を再開する。
const autoBackoffMax = 5 * time.Minute

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
	// ReportFailure records the internal stage and error for local diagnostics.
	// Detail exposed through AutoView remains a stable, secret-free code.
	ReportFailure func(stage string, err error)
	// Key は同期鍵を返す。vault がロック中または未設定なら false を返す。
	Key func() (string, bool)
	// Enabled は、この設置で自動同期が入っているかを返す。設定は保管庫の中に
	// あるので、閉じていれば false が返る。それで正しい。
	Enabled func() bool
	// Unattended は、自動処理による参照を vault の利用時刻に数えないようにする。
	Unattended func(run func())
	// Prepare restores the persisted binding after the vault is unlocked. It is
	// called before every user-requested or scheduled operation and must not
	// perform network I/O.
	Prepare func()

	interval time.Duration
	now      func() string
	// clock は、backoff と送信の期限を決める時計である。本番は time.Now のまま
	// 差し替えない。テストは SetClockForTest で替え、実時間の sleep に頼らずに
	// 期限の前後を進める。送信の期限を待つ timer も、テストでは同じ時計に合わせて
	// 発火させる（newPushTimer）。
	clock    func() time.Time
	pushWake chan struct{}
	// newPushTimer は、Run が送信の期限を待つ timer を作る。本番は time.Timer で、
	// テストは clock と一緒に進めて自分で発火させる timer に替える。
	newPushTimer func(delay time.Duration) pushTimer
	pushMutex    sync.Mutex
	pushDue      time.Time
	pending      bool

	// cycleMutex は、一巡が重ならないようにする。時計が来たときと、ユーザーが「今すぐ」を
	// 押したときが同時に起きうる。
	cycleMutex sync.Mutex

	mutex sync.Mutex
	view  AutoView
	// blockedETag is the remote generation which needs a human decision. A
	// ticker still performs HEAD, but does not download or derive a key again
	// until that generation changes or a manual Apply advances local state.
	blockedETag   string
	blockedTarget string
	blockedDetail string
	// failedETag is an unreadable remote generation. Retrying the same object
	// every minute would repeat both the download and password KDF without any
	// chance of a different result; a changed ETag clears the implicit cache key.
	failedTarget        string
	failedETag          string
	failedKeyID         string
	failedDetail        string
	failedDeterministic bool
	failedUntil         time.Time
	failedAttempts      int
}

// NewAuto は、巡回を組み立てる。走り出すのは Run が呼ばれてからである。
func NewAuto(service *Service, interval time.Duration, now func() string) *Auto {
	if interval <= 0 {
		interval = AutoInterval
	}
	return &Auto{
		service:      service,
		interval:     interval,
		now:          now,
		clock:        time.Now,
		pushWake:     make(chan struct{}, 1),
		newPushTimer: newSystemPushTimer,
		view:         AutoView{Phase: AutoIdle},
	}
}

// View は、画面へ渡す形の現在地。
func (a *Auto) View() AutoView {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	view := a.view
	view.Enabled = a.enabled()
	return view
}

func (a *Auto) enabled() bool { return a.Enabled != nil && a.Enabled() }

// Run は、ctx が終わるまで巡回し続ける。
func (a *Auto) Run(ctx context.Context) {
	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()
	schedule := pushSchedule{start: a.newPushTimer}
	defer schedule.stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.Poll(ctx)
		case <-a.pushWake:
			if delay, ok := a.pushWait(); ok {
				schedule.reset(delay)
			}
		case <-schedule.fired:
			if delay, due := a.takeScheduledPush(); !due {
				schedule.reset(delay)
				continue
			}
			schedule.clear()
			a.SendScheduled(ctx)
		}
	}
}

// NotifyLocalChange schedules one upload after local changes have been quiet
// for AutoPushDelay. Repeated notifications move the same deadline instead of
// creating one timer or snapshot per write.
func (a *Auto) NotifyLocalChange() {
	a.pushMutex.Lock()
	a.pending = true
	a.pushDue = a.clock().Add(AutoPushDelay)
	a.pushMutex.Unlock()
	select {
	case a.pushWake <- struct{}{}:
	default:
	}
}

func (a *Auto) pushWait() (time.Duration, bool) {
	a.pushMutex.Lock()
	defer a.pushMutex.Unlock()
	if !a.pending {
		return 0, false
	}
	return max(a.pushDue.Sub(a.clock()), 0), true
}

func (a *Auto) takeScheduledPush() (time.Duration, bool) {
	a.pushMutex.Lock()
	defer a.pushMutex.Unlock()
	if !a.pending {
		return 0, true
	}
	if wait := a.pushDue.Sub(a.clock()); wait > 0 {
		return wait, false
	}
	a.pending = false
	a.pushDue = time.Time{}
	return 0, true
}

// Poll performs only the receive half of automatic synchronization. If it
// observes local divergence, upload is scheduled through the debounce path.
func (a *Auto) Poll(ctx context.Context) AutoView {
	a.cycleMutex.Lock()
	defer a.cycleMutex.Unlock()
	if a.Unattended != nil {
		var view AutoView
		a.Unattended(func() { view = a.poll(ctx) })
		return view
	}
	return a.poll(ctx)
}

func (a *Auto) poll(ctx context.Context) AutoView {
	if !a.enabled() {
		return a.View()
	}
	a.prepare()
	a.service.operationMutex.Lock()
	syncKey, ok := a.currentSyncKey()
	if !ok {
		a.service.operationMutex.Unlock()
		return a.View()
	}
	a.enter(AutoRunning, "")
	phase, detail, done := a.receive(ctx, syncKey)
	shouldPush := false
	if !done && a.service.Direction() != DirectionPull {
		changed, err := a.service.diverged()
		if err != nil {
			phase, detail, done = AutoFailed, failureDetail(err), true
		} else {
			shouldPush = changed
		}
	}
	a.service.operationMutex.Unlock()
	if done {
		a.enter(phase, detail)
		return a.View()
	}
	a.enter(AutoIdle, "")
	if shouldPush {
		a.NotifyLocalChange()
	}
	return a.View()
}

// SendScheduled は、NotifyLocalChange で予約した送信を今行う。Run の timer が
// 呼ぶほか、テストは Poll と組で 1 巡（受信してから送信）を同期的に進める。
func (a *Auto) SendScheduled(ctx context.Context) AutoView {
	a.cycleMutex.Lock()
	defer a.cycleMutex.Unlock()
	if a.Unattended != nil {
		var view AutoView
		a.Unattended(func() { view = a.sendScheduledEnabled(ctx) })
		return view
	}
	return a.sendScheduledEnabled(ctx)
}

func (a *Auto) sendScheduledEnabled(ctx context.Context) AutoView {
	if !a.enabled() {
		return a.View()
	}
	a.prepare()
	a.service.operationMutex.Lock()
	defer a.service.operationMutex.Unlock()
	syncKey, ok := a.currentSyncKey()
	if !ok {
		return a.View()
	}
	a.enter(AutoRunning, "")
	phase, detail := a.send(ctx, syncKey)
	a.enter(phase, detail)
	return a.View()
}

// Now runs one user-requested cycle even when scheduled automatic sync is off.
func (a *Auto) Now(ctx context.Context) AutoView {
	a.cycleMutex.Lock()
	defer a.cycleMutex.Unlock()
	a.prepare()
	a.service.operationMutex.Lock()
	defer a.service.operationMutex.Unlock()
	// Read the key only after winning the same operation boundary as ReplaceKey.
	// Otherwise a waiter can retain the old key, observe the new ETag, and push
	// old-key ciphertext over the freshly rotated live object.
	syncKey, ok := a.currentSyncKey()
	if !ok {
		return a.View()
	}
	a.enter(AutoRunning, "")

	if phase, detail, done := a.receive(ctx, syncKey); done {
		a.enter(phase, detail)
		return a.View()
	}
	phase, detail := a.send(ctx, syncKey)
	a.enter(phase, detail)
	return a.View()
}

// ManualApplyCompleted clears a decision which was satisfied by an explicit
// preview-and-apply operation. Serialize with a running cycle so an older cycle
// cannot restore a stale blocked view after the apply has advanced local state.
func (a *Auto) ManualApplyCompleted() {
	a.cycleMutex.Lock()
	defer a.cycleMutex.Unlock()
	a.clearBlocked()
	a.clearFailed()
	a.enter(AutoIdle, "")
}

func (a *Auto) prepare() {
	if a.Prepare != nil {
		a.Prepare()
	}
}

func (a *Auto) currentSyncKey() (string, bool) {
	if a.Key == nil {
		return "", false
	}
	syncKey, ok := a.Key()
	return syncKey, ok && syncKey != ""
}

// receive はリモート更新を取り込む。done はこの巡回を終了すべきことを示す。
func (a *Auto) receive(ctx context.Context, syncKey string) (AutoPhase, string, bool) {
	if a.service.Direction() == DirectionPush {
		generation, err := a.service.inspectRemoteGeneration(ctx)
		if err != nil {
			return AutoFailed, failureDetail(err), true
		}
		if !generation.moved {
			a.clearBlocked()
			a.clearFailed()
			return AutoIdle, "", false
		}
		if generation.deleted {
			return AutoBlocked, "remote_deleted", true
		}
		if detail, ok := a.blocked(generation.target, generation.etag); ok {
			return AutoBlocked, detail, true
		}
		// A send-only installation cannot resolve a remote generation by
		// downloading it. Stop before creating a history candidate and wait for
		// an explicit force push or a direction change.
		a.rememberBlocked(generation.target, generation.etag, "remote_moved")
		return AutoBlocked, "remote_moved", true
	}
	// 動いていないものは取りに行かない。HEAD は ETag だけを返す。
	generation, err := a.service.inspectRemoteGeneration(ctx)
	if err != nil {
		return AutoFailed, failureDetail(err), true
	}
	if !generation.moved {
		a.clearBlocked()
		a.clearFailed()
		return AutoIdle, "", false
	}
	if generation.deleted {
		return AutoBlocked, "remote_deleted", true
	}
	if detail, ok := a.blocked(generation.target, generation.etag); ok {
		return AutoBlocked, detail, true
	}
	if detail, ok := a.failed(generation, syncKey); ok {
		return AutoFailed, detail, true
	}
	// 自動同期では競合の解決先を選ばない。
	result, err := a.service.pull(ctx, syncKey, ResolveNone, "")
	switch {
	case errors.Is(err, ErrNoSnapshot):
		return AutoIdle, "", false
	case errors.Is(err, ErrRemoteMoved):
		a.rememberBlocked(generation.target, generation.etag, "remote_moved")
		return AutoBlocked, "remote_moved", true
	case err != nil && !errors.Is(err, ErrNothingToApply):
		a.reportFailure("pull", err)
		detail := failureDetail(err)
		a.rememberFailed(generation, syncKey, err, detail)
		return AutoFailed, detail, true
	}
	a.clearFailed()
	if len(result.Conflicts) > 0 {
		a.rememberBlocked(result.target, result.ETag, "conflicts")
		return AutoBlocked, "conflicts", true
	}
	// 削除はユーザーの確認が必要なため自動適用しない。
	if len(result.Removed) > 0 {
		a.rememberBlocked(result.target, result.ETag, "removals")
		return AutoBlocked, "removals", true
	}
	// 差分がなくてもApplyはremote世代を記録する。これを省くと同じsnapshotを
	// 毎分取得し続け、次のpushも古いETagで拒否される。
	if err := a.service.validatePullForApply(ctx, result); err != nil {
		a.reportFailure("validate_apply", err)
		return AutoFailed, failureDetail(err), true
	}
	if err := a.service.apply(result); err != nil {
		if errors.Is(err, ErrApplyRefused) {
			return AutoIdle, "", false
		}
		a.reportFailure("apply", err)
		return AutoFailed, failureDetail(err), true
	}
	return AutoIdle, "", false
}

func (a *Auto) reportFailure(stage string, err error) {
	if a.ReportFailure != nil && err != nil {
		a.ReportFailure(stage, err)
	}
}

// send はローカルに変更があれば push する。
func (a *Auto) send(ctx context.Context, syncKey string) (AutoPhase, string) {
	if a.service.Direction() == DirectionPull {
		return AutoIdle, ""
	}
	// push の前に、中身を持たずに digest だけで変更の有無を見る。送信の予約は同期以外の
	// どの書き込みでも入るので、送るものの無い送信が多い。push を直接呼ぶと、そのたびに
	// ~/.ssh 全体を中身ごとメモリへ読むことになる。
	changed, err := a.service.diverged()
	if err != nil {
		return AutoFailed, failureDetail(err)
	}
	if !changed {
		return AutoIdle, ""
	}
	if _, err := a.service.push(ctx, syncKey, "", ""); err != nil {
		switch {
		case errors.Is(err, ErrPushRefused), errors.Is(err, ErrNothingToPush):
			return AutoIdle, ""
		case errors.Is(err, ErrRemoteMoved):
			// 次の巡回が先に受け取る。失敗ではなく、順番の問題である。
			return AutoIdle, ""
		}
		return AutoFailed, failureDetail(err)
	}
	return AutoIdle, ""
}

func (a *Auto) rememberBlocked(target, etag, detail string) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.blockedTarget = target
	a.blockedETag = etag
	a.blockedDetail = detail
}

func (a *Auto) blocked(target, etag string) (string, bool) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	return a.blockedDetail, target != "" && etag != "" &&
		a.blockedTarget == target && a.blockedETag == etag
}

func (a *Auto) clearBlocked() {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.blockedETag = ""
	a.blockedTarget = ""
	a.blockedDetail = ""
}

func (a *Auto) rememberFailed(generation remoteGeneration, syncKey string, cause error, detail string) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	keyID := Digest([]byte(syncKey))
	same := a.failedTarget == generation.target && a.failedETag == generation.etag && a.failedKeyID == keyID
	if same {
		a.failedAttempts++
	} else {
		a.failedAttempts = 1
	}
	a.failedTarget = generation.target
	a.failedETag = generation.etag
	// Store only a one-way identifier. Auto must notice a corrected key without
	// retaining the synchronization secret itself in another long-lived field.
	a.failedKeyID = keyID
	a.failedDetail = detail
	a.failedDeterministic = deterministicSyncFailure(cause)
	if a.failedDeterministic {
		a.failedUntil = time.Time{}
		return
	}
	a.failedUntil = a.clock().Add(backoffDelay(a.interval, a.failedAttempts))
}

// backoffDelayは、同じ世代への一時的な失敗がfailedAttempts回続いたあと、次に試すまで
// 待つ時間。巡回の間隔から倍に延ばし、autoBackoffMaxで止める。
func backoffDelay(interval time.Duration, failedAttempts int) time.Duration {
	delay := interval
	for attempt := 1; attempt < failedAttempts && delay < autoBackoffMax; attempt++ {
		delay *= 2
	}
	return min(delay, autoBackoffMax)
}

func (a *Auto) failed(generation remoteGeneration, syncKey string) (string, bool) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	match := generation.etag != "" &&
		a.failedTarget == generation.target && a.failedETag == generation.etag &&
		a.failedKeyID == Digest([]byte(syncKey))
	if !match {
		return "", false
	}
	if a.failedDeterministic || a.clock().Before(a.failedUntil) {
		return a.failedDetail, true
	}
	return "", false
}

func (a *Auto) clearFailed() {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.failedETag = ""
	a.failedTarget = ""
	a.failedKeyID = ""
	a.failedDetail = ""
	a.failedDeterministic = false
	a.failedUntil = time.Time{}
	a.failedAttempts = 0
}

// ResetRemoteCache is called after a successful configuration change. An ETag
// is meaningful only within one target, and credentials/direction can also turn
// a previously failed operation into a valid one.
func (a *Auto) ResetRemoteCache() {
	a.clearBlocked()
	a.clearFailed()
}

func deterministicSyncFailure(err error) bool {
	return errors.Is(err, ErrWrongPassphrase) || errors.Is(err, ErrUnsupportedEnvelopeVersion) ||
		errors.Is(err, ErrUnsupportedVersion) || errors.Is(err, ErrCostRefused) ||
		errors.Is(err, ErrUnsafePath) || errors.Is(err, ErrUnsafeMode) ||
		errors.Is(err, ErrManifestMismatch) || errors.Is(err, ErrNotASnapshot) ||
		errors.Is(err, ErrInvalidIgnoreRules)
}

func (a *Auto) enter(phase AutoPhase, detail string) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.view.Phase = phase
	a.view.Detail = detail
	if phase != AutoRunning {
		a.view.At = a.now()
	}
}

// failureDetail は、機密情報を含みうるエラー文を返さず、画面用の安定した code に変換する。
// HTTP の問題応答と同じ表（Classify）を使う。
func failureDetail(err error) string { return Classify(err).Code }
