package terminal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"hash/fnv"
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
	// Open は呼び出し側が Process を生成するコールバックである。
	Open func(ctx context.Context, size Size) (Process, error)
	// Reopen defaults to Open. Agent resume uses a one-shot Open while keeping
	// the ordinary shell opener for later transport/manual reconnects.
	Reopen func(ctx context.Context, size Size) (Process, error)
	Resume func(ctx context.Context, size Size, kind AgentKind, reference string) (Process, error)
	// ReplacementBusy marks startup automation which writes outside Session.Write.
	ReplacementBusy bool
	// ReconnectError は再接続失敗を分類する。retry=false なら即座に停止する。
	// problem は公開用の固定コードであり、raw error を含めない。
	ReconnectError func(error) (retry bool, problem string)
	Cleanup        func()
}

// Registry は、開いているセッションと、終了して残しているセッションを持つ。
type Registry struct {
	// Start は PTY を確保する。nil の場合はセッションを開始できない。
	Start  Starter
	Limits func() Limits
	// ReconnectDelay は、輸送が落ちたあと繋ぎ直すまでの間隔である。nil なら既定。
	ReconnectDelay func(attempt int) time.Duration
	// Reconnects は、繋ぎ直しを何回まで試みてよいかを返す。nil なら既定。
	Reconnects func() int
	// Now と Random は、テストが時計と ID を固定するためにここにある。
	Now    func() time.Time
	Random io.Reader

	mutex    sync.Mutex
	waiters  *sync.Cond
	sessions []*Session

	closing bool
	forced  bool

	pending           map[uint64]context.CancelFunc
	pendingReconnects int
	nextTicket        uint64
	// outstanding は、送り出したまま戻っていない graceful/force の呼び出し数。
	outstanding int
	joined      []error

	waiting bool
	waited  bool
	waitErr error
}

// condition は mutex に対応する条件変数を返す。
func (r *Registry) condition() *sync.Cond {
	if r.waiters == nil {
		r.waiters = sync.NewCond(&r.mutex)
	}
	return r.waiters
}

// reconnectDelay は、繋ぎ直しを待つ間隔である。
func (r *Registry) reconnectDelay(attempt int, sessionID string) time.Duration {
	if r.ReconnectDelay != nil {
		return r.ReconnectDelay(attempt)
	}
	return jitteredReconnectDelay(attempt, sessionID)
}

// jitteredReconnectDelay は、同時に切れたセッションの再接続を分散する。
// ランダムな session ID で集中を避けつつ、同じセッションの表示時刻は純粋計算で
// 安定させる。
func jitteredReconnectDelay(attempt int, sessionID string) time.Duration {
	base := reconnectBackoff[min(attempt, len(reconnectBackoff)-1)]
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(sessionID))
	_, _ = hash.Write([]byte{byte(attempt), byte(attempt >> 8)})
	percent := 80 + int(hash.Sum32()%41)
	return base * time.Duration(percent) / 100
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

// liveCount は、確保中または生存中のセッション数を重複なく返す。
// 手動再接続は既存セッションを生存状態へ戻してから pending にも載るため、
// pendingReconnects の分だけ二重計上を除く。
func (r *Registry) liveCount() int {
	live := len(r.pending) - r.pendingReconnects
	for _, session := range r.sessions {
		if session.Live() {
			live++
		}
	}
	return live
}

// Open は PTY をひとつ確保し、その中でプログラムを起動する。
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
	if r.liveCount() >= limits.MaxSessions {
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
	// 予約解除を待機中の停止処理へ通知する。
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
		// 停止開始後に返った Process は公開せず、ここで終了する。
		r.discard(process, spec.Cleanup)
		return nil, ErrShuttingDown
	}

	session := &Session{
		id: id, kind: spec.Kind, alias: spec.Alias, title: spec.Title,
		fallbackTitle: spec.Title,
		started:       r.now(),
		buffer:        NewRing(limits.Scrollback),
		streams:       map[*Stream]bool{},
		process:       process,
		cleanup:       spec.Cleanup,
		done:          make(chan struct{}),
		// 繋ぎ直せるのは、開き方を知っているセッションだけである。
		reopen:          spec.Reopen,
		resume:          spec.Resume,
		replacementBusy: spec.ReplacementBusy,
		reconnectError:  spec.ReconnectError,
		size:            size,
		stopping:        make(chan struct{}),
		state:           StateConnecting,
		delay: func(attempt int) time.Duration {
			return r.reconnectDelay(attempt, id)
		},
		attempts: r.reconnects,
		now:      r.now,
	}
	if spec.Alias != "" {
		session.titleSource = TitleConnection
	} else {
		session.titleSource = TitleFallback
	}
	session.observeProcess(StateConnecting, "")
	if spec.Reopen == nil {
		session.reopen = spec.Open
	}
	r.mutex.Lock()
	r.sessions = append(r.sessions, session)
	r.prune()
	r.mutex.Unlock()
	go session.pump(r.now)
	return session, nil
}

// reserve は確保処理へ識別子を割り当てる。呼び出し側が mutex を保持する。
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
func (r *Registry) prune() {
	exited := 0
	for index := len(r.sessions) - 1; index >= 0; index-- {
		if r.sessions[index].Live() {
			continue
		}
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

func (r *Registry) MaxSessions() int { return r.limits().MaxSessions }

// Sessions は、生存と終了済みの両方を、開いた順に返す。
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

// CommandTarget resolves only an existing live session. It never treats the ID
// as an SSH alias and therefore cannot create a new connection as a fallback.
func (r *Registry) CommandTarget(id string) (CommandTarget, error) {
	session, ok := r.Lookup(id)
	if !ok {
		return CommandTarget{}, ErrNotFound
	}
	return session.CommandTarget()
}

// WriteCommand writes to the exact generation captured by CommandTarget.
func (r *Registry) WriteCommand(ctx context.Context, target CommandTarget, command string) error {
	return r.WriteCommandInput(ctx, target, command, true)
}

// WriteCommandInput writes to the exact generation and optionally appends Enter.
func (r *Registry) WriteCommandInput(ctx context.Context, target CommandTarget, command string, submit bool) error {
	session, ok := r.Lookup(target.ID)
	if !ok {
		return ErrNotFound
	}
	return session.WriteCommandInput(ctx, target.Generation, command, submit)
}

// Reconnect は終了済みのSSHセッションを同じIDとscrollbackのまま開き直す。
// 新規Openと同じ上限に数え、closeやengine停止が先行したProcessは公開しない。
func (r *Registry) Reconnect(ctx context.Context, id string) (*Session, error) {
	limits := r.limits()
	r.mutex.Lock()
	if r.closing {
		r.mutex.Unlock()
		return nil, ErrShuttingDown
	}
	var session *Session
	for _, candidate := range r.sessions {
		if candidate.id == id {
			session = candidate
			break
		}
	}
	if session == nil {
		r.mutex.Unlock()
		return nil, ErrNotFound
	}
	if r.liveCount() >= limits.MaxSessions {
		r.mutex.Unlock()
		return nil, ErrSessionLimit
	}
	reopen, size, previous, err := session.prepareManualReconnect()
	if err != nil {
		r.mutex.Unlock()
		return nil, err
	}
	creation, cancel := context.WithCancel(ctx)
	ticket := r.reserve(cancel)
	r.pendingReconnects++
	r.mutex.Unlock()

	process, openErr := reopen(creation, size)

	r.mutex.Lock()
	delete(r.pending, ticket)
	r.pendingReconnects--
	r.condition().Broadcast()
	lost := r.closing || creation.Err() != nil
	r.mutex.Unlock()
	cancel()

	if openErr != nil {
		session.failManualReconnect(previous, session.manualReconnectProblem(openErr))
		return nil, openErr
	}
	if lost || !session.completeManualReconnect(process, r.now()) {
		r.discard(process, nil)
		session.failManualReconnect(previous, "")
		if lost {
			return nil, ErrShuttingDown
		}
		return nil, ErrReconnectUnavailable
	}
	go session.pump(r.now)
	return session, nil
}

type AgentResumePlacement string

const (
	AgentResumeSamePane AgentResumePlacement = "same-pane"
	AgentResumeNewPane  AgentResumePlacement = "new-pane"
)

// ResumeAgent starts only the fixed program selected by the candidate's
// adapter. The opaque reference remains inside terminal.Session/Registry.
func (r *Registry) ResumeAgent(ctx context.Context, id string, version uint64, placement AgentResumePlacement) (*Session, error) {
	source, ok := r.Lookup(id)
	if !ok {
		return nil, ErrNotFound
	}
	plan, err := source.agentResumePlan(version)
	if err != nil {
		return nil, err
	}
	if placement == AgentResumeNewPane {
		title := source.alias
		if plan.candidate.name != "" {
			title = plan.candidate.name
		}
		opened, openErr := r.Open(ctx, Spec{
			Kind: source.kind, Alias: source.alias, Title: title, Size: plan.size,
			Open: func(ctx context.Context, size Size) (Process, error) {
				return plan.open(ctx, size, plan.candidate.kind, plan.candidate.reference)
			},
			Reopen:         source.reopen,
			Resume:         source.resume,
			ReconnectError: source.reconnectError,
		})
		if openErr != nil {
			return nil, openErr
		}
		current, currentErr := source.agentResumePlan(version)
		if currentErr != nil || current.candidate.kind != plan.candidate.kind || current.candidate.reference != plan.candidate.reference {
			_ = r.Close(opened.ID())
			return nil, ErrAgentResumeStale
		}
		opened.seedAgentCandidate(plan.candidate)
		return opened, nil
	}
	if placement != AgentResumeSamePane {
		return nil, ErrAgentResumeUnavailable
	}
	if plan.busy {
		return nil, ErrAgentResumeSamePaneBusy
	}
	if plan.live {
		done := source.Done()
		if err := source.forceClose(); err != nil {
			return nil, err
		}
		select {
		case <-done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		var err error
		version, err = source.agentResumeVersionFor(plan.candidate)
		if err != nil {
			return nil, err
		}
	}
	return r.resumeAgentSamePane(ctx, source, version)
}

func (r *Registry) resumeAgentSamePane(ctx context.Context, session *Session, version uint64) (*Session, error) {
	limits := r.limits()
	r.mutex.Lock()
	if r.closing {
		r.mutex.Unlock()
		return nil, ErrShuttingDown
	}
	if r.liveCount() >= limits.MaxSessions {
		r.mutex.Unlock()
		return nil, ErrSessionLimit
	}
	open, size, candidate, previous, err := session.prepareAgentResume(version)
	if err != nil {
		r.mutex.Unlock()
		return nil, err
	}
	creation, cancel := context.WithCancel(ctx)
	ticket := r.reserve(cancel)
	r.pendingReconnects++
	r.mutex.Unlock()

	process, openErr := open(creation, size, candidate.kind, candidate.reference)
	r.mutex.Lock()
	delete(r.pending, ticket)
	r.pendingReconnects--
	r.condition().Broadcast()
	lost := r.closing || creation.Err() != nil
	r.mutex.Unlock()
	cancel()

	if openErr != nil {
		session.failManualReconnect(previous, session.manualReconnectProblem(openErr))
		return nil, openErr
	}
	if lost || !session.completeAgentResume(process, candidate, r.now()) {
		r.discard(process, nil)
		session.failManualReconnect(previous, "")
		if lost {
			return nil, ErrShuttingDown
		}
		return nil, ErrAgentResumeUnavailable
	}
	go session.pump(r.now)
	return session, nil
}

// Rename はセッションの表示名を変える。
func (r *Registry) Rename(id, title string) error {
	session, ok := r.Lookup(id)
	if !ok {
		return ErrNotFound
	}
	return session.Rename(title)
}

// UnpinTitle lets the current agent name or connection fallback drive the
// display title again.
func (r *Registry) UnpinTitle(id string) error {
	session, ok := r.Lookup(id)
	if !ok {
		return ErrNotFound
	}
	session.UnpinTitle()
	return nil
}

// Close は、利用者が閉じると決めたセッションを強制停止し、一覧から消す。
func (r *Registry) Close(id string) error {
	session, ok := r.Lookup(id)
	if !ok {
		return ErrNotFound
	}
	session.Discard()
	session.closeStreams()
	if session.Live() {
		if err := session.forceClose(); err != nil {
			return err
		}
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

// BeginShutdown は、停止を頼むだけである。何も待たない。
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

// ForceClose は、締切に達した合図である。これも何も待たない。
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

// Wait は全停止処理の完了を待つ。
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
