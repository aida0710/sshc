package terminal

import (
	"crypto/rand"
	"encoding/hex"
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
	// Now と Random は、テストが時計と ID を固定するためにここにある。
	Now    func() time.Time
	Random io.Reader

	mutex    sync.Mutex
	sessions []*Session
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
// 生存上限に達していれば拒否する。黙って古いセッションを閉じることはしない。
// PTY を確保できなければセッションを作らずに理由を返す——作ってしまえば、
// 何も起きていないセッションが一覧に並ぶことになる。
func (r *Registry) Open(spec Spec) (*Session, error) {
	limits := r.limits()

	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.Start == nil {
		return nil, ErrNoStarter
	}
	live := 0
	for _, session := range r.sessions {
		if session.Live() {
			live++
		}
	}
	if live >= limits.MaxSessions {
		return nil, ErrSessionLimit
	}

	id, err := r.mintID()
	if err != nil {
		return nil, err
	}
	size := spec.Size
	if !size.Valid() {
		size = Size{Cols: 80, Rows: 24}
	}
	process, err := r.Start.Start(spec.Command, size)
	if err != nil {
		if spec.Cleanup != nil {
			spec.Cleanup()
		}
		return nil, err
	}

	session := &Session{
		id: id, kind: spec.Kind, alias: spec.Alias, title: spec.Title,
		started: r.now(),
		buffer:  NewRing(limits.Scrollback),
		streams: map[*Stream]bool{},
		process: process,
		cleanup: spec.Cleanup,
		done:    make(chan struct{}),
	}
	r.sessions = append(r.sessions, session)
	r.prune()
	go session.pump(r.now)
	return session, nil
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
func (r *Registry) prune() {
	exited := 0
	for index := len(r.sessions) - 1; index >= 0; index-- {
		if r.sessions[index].Live() {
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
func (r *Registry) Sessions() []View {
	r.mutex.Lock()
	defer r.mutex.Unlock()
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

// Shutdown は、生きているすべてのセッションへ SIGHUP を送る。
//
// アプリケーションが終了するときの後始末である。待たないのは、応答しない
// リモートに向いた ssh のために終了が引き延ばされてはならないからだ。
func (r *Registry) Shutdown() {
	r.mutex.Lock()
	sessions := append([]*Session(nil), r.sessions...)
	r.mutex.Unlock()
	for _, session := range sessions {
		_ = session.Hangup()
	}
}
