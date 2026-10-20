// Package connectionattempt は、HTTP の要求の中で張る接続（SFTP のタブが最初の一覧を取る
// 要求など）が接続するあいだに報告した段階と接続ログを預かる。要求は接続し終わるまで
// 返らないので、Web UI はこれを別の要求で読み、待っているあいだの経過を表示する。
package connectionattempt

import (
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"sshc/internal/terminal"
)

const (
	// maxAttempts は、同時に預かる試行の数の上限である。識別子は Web UI が決めるので、
	// 上限が無いと要求のたびに増え続ける。タブ8つ×ペイン2つより十分に大きくしてある。
	maxAttempts = 64
	// maxLogLines は、1つの試行で預かる接続ログの行数の上限である。-vvv の1接続は数十行で、
	// ProxyJump を重ねても収まる。超えた分は古い行から捨てる。
	maxLogLines = 400
	// finishedRetention は、終わった試行を預かっておく時間である。要求が返ったあとに
	// Web UI が最後の行を読みに来るのを待つ。
	finishedRetention = 2 * time.Minute
)

// ErrInvalidID は、試行の識別子が決まった形でないことを表す。
var ErrInvalidID = errors.New("connection attempt id must be 16 to 64 letters, digits or hyphens")

// idPattern は、Web UI が作る UUID を受け付け、パスや区切りに使う文字を受け付けない形である。
var idPattern = regexp.MustCompile(`^[A-Za-z0-9-]{16,64}$`)

// Snapshot は、ある時点の試行の様子である。
type Snapshot struct {
	// Progress は最後に報告された段階で、まだ何も報告されていなければ nil である。
	Progress *terminal.ConnectionProgress
	// Log は接続ログの行で、行末の改行を含まない。
	Log      []string
	Finished bool
}

// Registry は、進行中と終わって間もない試行を alias と識別子で預かる。
type Registry struct {
	mutex    sync.Mutex
	attempts map[attemptKey]*Attempt
	now      func() time.Time
}

type attemptKey struct{ alias, id string }

// NewRegistry は、空の Registry を返す。
func NewRegistry() *Registry {
	return &Registry{attempts: map[attemptKey]*Attempt{}, now: time.Now}
}

// Begin は、alias への接続の試行を識別子 id で預かり始める。同じ識別子の試行が
// 既にあれば、新しい試行で置き換える。
func (r *Registry) Begin(alias, id string) (*Attempt, error) {
	if !idPattern.MatchString(id) {
		return nil, ErrInvalidID
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	now := r.now()
	r.pruneLocked(now)
	key := attemptKey{alias: alias, id: id}
	if _, exists := r.attempts[key]; !exists && len(r.attempts) >= maxAttempts {
		r.evictOldestLocked()
	}
	attempt := &Attempt{registry: r, started: now}
	r.attempts[key] = attempt
	return attempt, nil
}

// Snapshot は、alias への識別子 id の試行の様子を返す。知らない試行なら false を返す。
// 要求が試行を預ける前に読みに来ることもあるので、Web UI は false を「まだ」と読む。
func (r *Registry) Snapshot(alias, id string) (Snapshot, bool) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.pruneLocked(r.now())
	attempt, ok := r.attempts[attemptKey{alias: alias, id: id}]
	if !ok {
		return Snapshot{}, false
	}
	snapshot := Snapshot{Log: append([]string(nil), attempt.lines...), Finished: attempt.finished}
	if attempt.progress != nil {
		progress := *attempt.progress
		snapshot.Progress = &progress
	}
	return snapshot, true
}

func (r *Registry) pruneLocked(now time.Time) {
	for key, attempt := range r.attempts {
		if attempt.finished && now.Sub(attempt.finishedAt) > finishedRetention {
			delete(r.attempts, key)
		}
	}
}

func (r *Registry) evictOldestLocked() {
	var oldestKey attemptKey
	var oldest *Attempt
	for key, attempt := range r.attempts {
		if oldest == nil || attempt.started.Before(oldest.started) {
			oldestKey, oldest = key, attempt
		}
	}
	delete(r.attempts, oldestKey)
}

// Attempt は1つの接続の試行である。Progress で段階を、Write で接続ログを受け取り、
// Finish で終わる。どれも複数の goroutine から呼んでよい。
type Attempt struct {
	registry   *Registry
	started    time.Time
	finishedAt time.Time
	finished   bool
	progress   *terminal.ConnectionProgress
	lines      []string
	// partial は、まだ改行が届いていない行の頭である。
	partial string
}

// Progress は、接続の段階を記録する。
func (a *Attempt) Progress(progress terminal.ConnectionProgress) {
	a.registry.mutex.Lock()
	defer a.registry.mutex.Unlock()
	a.progress = &progress
}

// Write は、接続ログを受け取る。行は LF で区切り、行末の CR は捨てる。
func (a *Attempt) Write(contents []byte) (int, error) {
	a.registry.mutex.Lock()
	defer a.registry.mutex.Unlock()
	text := a.partial + string(contents)
	complete := strings.Split(text, "\n")
	a.partial = complete[len(complete)-1]
	for _, line := range complete[:len(complete)-1] {
		a.appendLineLocked(line)
	}
	return len(contents), nil
}

// Finish は、試行を終える。err があれば、端末の接続と同じく `sshc: ` を付けてログに残す。
func (a *Attempt) Finish(err error) {
	a.registry.mutex.Lock()
	defer a.registry.mutex.Unlock()
	if a.partial != "" {
		a.appendLineLocked(a.partial)
		a.partial = ""
	}
	if err != nil {
		for _, line := range strings.Split("sshc: "+err.Error(), "\n") {
			a.appendLineLocked(line)
		}
	}
	a.finished = true
	a.finishedAt = a.registry.now()
}

func (a *Attempt) appendLineLocked(line string) {
	a.lines = append(a.lines, strings.TrimRight(line, "\r"))
	if overflow := len(a.lines) - maxLogLines; overflow > 0 {
		a.lines = append([]string(nil), a.lines[overflow:]...)
	}
}
