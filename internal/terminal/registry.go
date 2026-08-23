package terminal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"sync"
	"time"
)

// Spec は、開こうとしているセッションひとつ分の要求である。
type Spec struct {
	Kind  Kind
	Alias string
	// Title は一覧に出す名前である。ssh は alias、shell はシェルの basename。
	Title   string
	Command Command
	Size    Size
	// Open は、このセッションの Process を呼び出し側が自分で作る継ぎ目である。
	//
	// 設定されていれば Starter は使われない。プロセス内で話す SSH がここを通る
	// ——向こうにプロセスが無いので、確保する PTY も無い。このパッケージが
	// SSH を知らないままでいられるのは、知る必要のあるものが全部この関数の
	// 内側にあるからである。
	//
	// context は確保だけを支配する。返った Process にそれを持たせてはならない。
	Open func(ctx context.Context, size Size) (Process, error)
	// Cleanup は子プロセスが終わったあとに一度だけ呼ばれる。凍結した ssh 設定の
	// ような、その接続のためだけに作られたものを片付けるためにある。
	Cleanup func()
}

// Registry は、開いているセッションと、終了して残しているセッションを持つ。
//
// PTY は常駐プロセス側で存続する。ブラウザのタブを閉じてもリロードしても
// セッションは生きている。終わるのは、子プロセスが終了したとき、人が閉じたとき、
// アプリケーションが終了したときだけである。
type Registry struct {
	// Start は PTY を確保する継ぎ目である。nil のレジストリは何も開かない。
	Start Starter
	// Limits は metadata が運ぶ上限を読む。読むのは開くときだけなので、設定を
	// 変えても、すでに開いているセッションが閉じられることはない。nil なら既定。
	Limits func() Limits
	// ReconnectDelay は、輸送が落ちたあと繋ぎ直すまでの間隔である。nil なら既定。
	ReconnectDelay func(attempt int) time.Duration
	// Reconnects は、繋ぎ直しを何回まで試みてよいかを答える。nil なら既定。
	//
	// **一度だけ読まない。** 設定は走っているあいだに変えられる——0 にした人が
	// 待たされるのは、いま粘っているセッションが諦めるまでであり、**それが
	// まさに 0 にした理由である。**
	Reconnects func() int
	// Now と Random は、テストが時計と ID を固定するためにここにある。
	Now    func() time.Time
	Random io.Reader

	mutex    sync.Mutex
	waiters  *sync.Cond
	sessions []*Session

	// closing と forced は、停止の二段階である。closing は新しいセッションを
	// 断り、forced は強制停止をもう始めたことを言う。どちらも戻らない。
	closing bool
	forced  bool

	// pending は、まだ Process を返していない Open の予約である。鍵は札で、
	// 値はその確保を取り消す関数である。**レジストリの錠を握ったまま外部の
	// 確保を呼ばない**ので、停止処理はこれを通してしか届かない。
	pending    map[uint64]context.CancelFunc
	nextTicket uint64
	// outstanding は、送り出したまま戻っていない graceful/force の呼び出し数。
	outstanding int
	joined      []error

	waiting bool
	waited  bool
	waitErr error
}

// condition は、この錠に結び付いた合図を返す。停止まで誰も待たないので、
// 最初に必要になったときに作る。
func (r *Registry) condition() *sync.Cond {
	if r.waiters == nil {
		r.waiters = sync.NewCond(&r.mutex)
	}
	return r.waiters
}

// reconnectDelay は、繋ぎ直しを待つ間隔である。
//
// **試験のために開いている。** 5 回の再試行に 33 秒かかる既定のままでは、
// 諦めるところを確かめる試験が現実の時間を待つことになる。
func (r *Registry) reconnectDelay(attempt int) time.Duration {
	if r.ReconnectDelay != nil {
		return r.ReconnectDelay(attempt)
	}
	return reconnectBackoff[min(attempt, len(reconnectBackoff)-1)]
}

// reconnects は、繋ぎ直しを何回まで試みるかである。
func (r *Registry) reconnects() int {
	if r.Reconnects == nil {
		return MaxReconnects
	}
	return NormaliseReconnects(r.Reconnects())
}

func (r *Registry) now() time.Time {
	if r.Now == nil {
		return time.Now()
	}
	return r.Now()
}

func (r *Registry) limits() Limits {
	if r.Limits == nil {
		return DefaultLimits()
	}
	return r.Limits().Normalise()
}

func (r *Registry) random() io.Reader {
	if r.Random == nil {
		return rand.Reader
	}
	return r.Random
}

// Open は PTY をひとつ確保し、その中でプログラムを起こす。
//
// ctx は**確保だけ**を支配する。成功して返った Process はそこから切り離された
// 寿命を持つ——さもなければ、開いた HTTP ハンドラが返った瞬間にセッションが死ぬ。
//
// 生存上限に達していれば拒否する。黙って古いセッションを閉じることはしない。
// PTY を確保できなければセッションを作らずに理由を返す——作ってしまえば、
// 何も起きていないセッションが一覧に並ぶことになる。
//
// **確保はレジストリの錠の外で走る。** 応答しないリモートへの接続がここで
// 止まっても、停止処理は錠を取れる。止まっている確保には、予約した取り消しを
// 通して届く。
func (r *Registry) Open(ctx context.Context, spec Spec) (*Session, error) {
	limits := r.limits()

	r.mutex.Lock()
	if r.closing {
		r.mutex.Unlock()
		return nil, ErrShuttingDown
	}
	if r.Start == nil && spec.Open == nil {
		r.mutex.Unlock()
		return nil, ErrNoStarter
	}
	live := len(r.pending)
	for _, session := range r.sessions {
		if session.Live() {
			live++
		}
	}
	if live >= limits.MaxSessions {
		r.mutex.Unlock()
		return nil, ErrSessionLimit
	}
	id, err := r.mintID()
	if err != nil {
		r.mutex.Unlock()
		return nil, err
	}
	creation, cancel := context.WithCancel(ctx)
	ticket := r.reserve(cancel)
	r.mutex.Unlock()

	size := spec.Size
	if !size.Valid() {
		size = Size{Cols: 80, Rows: 24}
	}
	open := spec.Open
	if open == nil {
		open = func(ctx context.Context, size Size) (Process, error) {
			return r.Start.Start(ctx, spec.Command, size)
		}
	}
	process, err := open(creation, size)

	r.mutex.Lock()
	delete(r.pending, ticket)
	// 予約を手放したことは、停止処理が待っている事実である。
	r.condition().Broadcast()
	lost := r.closing || creation.Err() != nil
	r.mutex.Unlock()
	cancel()

	if err != nil {
		if spec.Cleanup != nil {
			spec.Cleanup()
		}
		if lost && err == nil {
			return nil, ErrShuttingDown
		}
		return nil, err
	}
	if lost {
		// 遅れて返ってきた Process は、公開せずにここで畳む。一覧に載せてから
		// 閉じると、停止処理が数え終えた後に現れたことになる。
		r.discard(process, spec.Cleanup)
		return nil, ErrShuttingDown
	}

	session := &Session{
		id: id, kind: spec.Kind, alias: spec.Alias, title: spec.Title,
		started: r.now(),
		buffer:  NewRing(limits.Scrollback),
		streams: map[*Stream]bool{},
		process: process,
		cleanup: spec.Cleanup,
		done:    make(chan struct{}),
		// **繋ぎ直せるのは、開き方を知っているセッションだけである。**
		// ローカルのシェルには落ちる輸送が無いので、Spec.Open を持たない。
		reopen:   spec.Open,
		size:     size,
		stopping: make(chan struct{}),
		delay:    r.reconnectDelay,
		attempts: r.reconnects,
	}
	r.mutex.Lock()
	r.sessions = append(r.sessions, session)
	r.prune()
	r.mutex.Unlock()
	go session.pump(r.now)
	return session, nil
}

// reserve は札を配る。呼び出し側が錠を握っている。
func (r *Registry) reserve(cancel context.CancelFunc) uint64 {
	if r.pending == nil {
		r.pending = map[uint64]context.CancelFunc{}
	}
	r.nextTicket++
	ticket := r.nextTicket
	r.pending[ticket] = cancel
	return ticket
}

// discard は、公開しないと決めた Process を強制停止して回収する。
func (r *Registry) discard(process Process, cleanup func()) {
	if forcer, ok := process.(forceCloser); ok {
		_ = forcer.ForceClose()
	}
	_ = process.Close()
	process.Wait()
	if cleanup != nil {
		cleanup()
	}
}

// dispatch は、外部の呼び出しひとつを送り出し、戻るまでを数える。
//
// **錠を握ったまま外部を呼ばない。** 返らない Hangup ひとつが、他のすべての
// 停止と締切そのものを止めてしまう。
func (r *Registry) dispatch(call func() error) {
	r.mutex.Lock()
	r.outstanding++
	r.mutex.Unlock()
	go func() {
		err := call()
		r.mutex.Lock()
		if err != nil {
			r.joined = append(r.joined, err)
		}
		r.outstanding--
		r.condition().Broadcast()
		r.mutex.Unlock()
	}()
}

// mintID は、この一覧の中で衝突しない識別子を作る。
//
// alias ではない。同じ alias に何本でも開けるので、alias は名前であって
// 識別子ではない。
func (r *Registry) mintID() (string, error) {
	taken := make(map[string]bool, len(r.sessions))
	for _, session := range r.sessions {
		taken[session.id] = true
	}
	raw := make([]byte, 16)
	for attempt := 0; attempt < 8; attempt++ {
		if _, err := io.ReadFull(r.random(), raw); err != nil {
			return "", err
		}
		id := hex.EncodeToString(raw)
		if !taken[id] {
			return id, nil
		}
	}
	return "", ErrSessionLimit
}

// prune は、残す終了済みセッションを新しい方から RetainedExited 本までにする。
// 生きているセッションは決して捨てない。
//
// **人が自分で閉じたものは、数に入れず即座に捨てる。** 残す枠は「読まれて
// いない終わり方」——勝手に切れた、相手が落ちた——のためにある。閉じた人は
// もう読んでいて、そのうえで閉じている。
func (r *Registry) prune() {
	exited := 0
	for index := len(r.sessions) - 1; index >= 0; index-- {
		if r.sessions[index].Live() {
			continue
		}
		// **人が閉じたものは数に入れず、すぐ捨てる。** 残す枠は「読まれて
		// いない終わり方」のためにある。
		if r.sessions[index].Discarded() {
			r.sessions = append(r.sessions[:index], r.sessions[index+1:]...)
			continue
		}
		exited++
		if exited <= RetainedExited {
			continue
		}
		r.sessions = append(r.sessions[:index], r.sessions[index+1:]...)
	}
}

// Prune は、終了したセッションが増えたあとに呼ばれる公開の入り口である。
func (r *Registry) Prune() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.prune()
}

// MaxSessions は、いま効いている生存上限を返す。画面が「あと何本開けるか」を
// 自分で数えられるようにするためだけにある。
func (r *Registry) MaxSessions() int { return r.limits().MaxSessions }

// Sessions は、生存と終了済みの両方を、開いた順に返す。
//
// **数える前に片付ける。** 人が閉じたコンソールは、終わった時点で捨ててよい
// ——捨てる合図をどこか別の場所から鳴らすより、読む側が来たときに片付ける方が
// 確実である。終わりを観測するのは pump であり、あれは registry を知らない。
func (r *Registry) Sessions() []View {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.prune()
	views := make([]View, 0, len(r.sessions))
	for _, session := range r.sessions {
		views = append(views, session.View())
	}
	return views
}

// Lookup は識別子でセッションを引く。
func (r *Registry) Lookup(id string) (*Session, bool) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	for _, session := range r.sessions {
		if session.id == id {
			return session, true
		}
	}
	return nil, false
}

// Close は、生存中なら子プロセスに SIGHUP を送り、終了済みなら一覧から消す。
// Rename は、一覧に出す名前を変える。
//
// 名前は表示だけのもので、この一覧の中で一意である必要は無い。同じ名前を
// 二つ付けられるのは、どちらが何かを決めるのは利用者だからである。
func (r *Registry) Rename(id, title string) error {
	session, ok := r.Lookup(id)
	if !ok {
		return ErrNotFound
	}
	return session.Rename(title)
}

func (r *Registry) Close(id string) error {
	session, ok := r.Lookup(id)
	if !ok {
		return ErrNotFound
	}
	if session.Live() {
		// **自分で閉じたものは、一覧に残さない。** 終了済みを残すのは最後の
		// 出力を読ませるためであり、閉じた人はもう読んでいる。
		session.Discard()
		return session.Hangup()
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	for index, candidate := range r.sessions {
		if candidate.id == id {
			r.sessions = append(r.sessions[:index], r.sessions[index+1:]...)
			break
		}
	}
	return nil
}

// BeginShutdown は、停止を頼むだけである。**何も待たない。**
//
// 新しいセッションを断り、アタッチを外し、確保中のものを取り消し、生きている
// セッションへ SIGHUP を送り出す。送り出したものが返るのを待たないのは、
// 応答しないリモートに向いた ssh ひとつが、他のすべての停止と締切そのものを
// 止めてしまうからである。待つのは Wait だけである。
func (r *Registry) BeginShutdown() {
	r.mutex.Lock()
	if r.closing {
		r.mutex.Unlock()
		return
	}
	r.closing = true
	sessions := append([]*Session(nil), r.sessions...)
	cancels := make([]context.CancelFunc, 0, len(r.pending))
	for _, cancel := range r.pending {
		cancels = append(cancels, cancel)
	}
	r.mutex.Unlock()

	for _, cancel := range cancels {
		cancel()
	}
	for _, session := range sessions {
		session.closeStreams()
	}
	for _, session := range sessions {
		r.dispatch(session.Hangup)
	}
}

// ForceClose は、締切に達した合図である。**これも何も待たない。**
//
// graceful が返らないことこそが、これが呼ばれる理由である。ひとつの強制停止を
// 待ってから次を始めれば、返らない一本のために残り全部が止まる——それは
// 直したはずの欠陥そのものである。
func (r *Registry) ForceClose() {
	r.mutex.Lock()
	if r.forced {
		r.mutex.Unlock()
		return
	}
	r.forced = true
	r.closing = true
	sessions := append([]*Session(nil), r.sessions...)
	cancels := make([]context.CancelFunc, 0, len(r.pending))
	for _, cancel := range r.pending {
		cancels = append(cancels, cancel)
	}
	r.mutex.Unlock()

	for _, cancel := range cancels {
		cancel()
	}
	for _, session := range sessions {
		r.dispatch(session.forceClose)
	}
}

// Wait は、唯一の合流点である。
//
// 確保中のもの、送り出した graceful/force、そして登録済みのすべてのセッションの
// 終了を待つ。**ForceClose が Wait の後から仕事を足す**ので、数え上げは
// 「一度ゼロになったら終わり」ではいけない。締切とちょうど同時に最後の
// セッションが終わると、それだけでロックを手放して、まだ走っている強制停止と
// 状態変更が重なってしまう。
//
// 強制停止のあとも待ち続けるのは、壊れたアダプタのために早く戻って 2 台目の
// エンジンを許すより、ロックを握ったままの方が安全だからである。
func (r *Registry) Wait() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.waited {
		return r.waitErr
	}
	if r.waiting {
		for !r.waited {
			r.condition().Wait()
		}
		return r.waitErr
	}
	r.waiting = true

	for {
		for r.outstanding > 0 || len(r.pending) > 0 {
			r.condition().Wait()
		}
		sessions := append([]*Session(nil), r.sessions...)
		r.mutex.Unlock()
		for _, session := range sessions {
			<-session.done
		}
		r.mutex.Lock()
		if r.outstanding == 0 && len(r.pending) == 0 && len(r.sessions) == len(sessions) {
			break
		}
	}

	r.waitErr = errors.Join(r.joined...)
	r.waited = true
	r.condition().Broadcast()
	return r.waitErr
}
