package remoteengine

import (
	"errors"
	"sync"

	"sshc/internal/terminal"
)

// actionOutputCapacity は、操作ひとつについて、接続先の sshc の出力を覚えておく量である。
// sshc update が書くのは数十行なので、画面が読みに来る前に押し出されることはない。
// 上限を置くのは、接続先の sshc が延々と書いても、このマシンのメモリを使い切らないためである。
const actionOutputCapacity = 64 << 10

var (
	// ErrActionRunning は、同じ接続先で別の操作がまだ走っていることを表す。
	ErrActionRunning = errors.New("another action is still running on the remote engine")
	// ErrNoAction は、このエンジンが起動してから、接続先で操作を始めていないことを表す。
	ErrNoAction = errors.New("no action has been started on the remote engine")
	// ErrActionCursorAhead は、読む位置が、操作の出力の終わりより先にあることを表す。
	// 別の操作の位置を渡されたときに、これからの出力を黙って読み飛ばさないために断る。
	ErrActionCursorAhead = errors.New("the cursor is past the end of the action output")
	// ErrActionTimedOut は、操作が actionLimit までに終わらなかったことを表す。
	ErrActionTimedOut = errors.New("the action did not finish within its time limit")
)

// ActionLogRead は、実行先のエンジンへの最後の操作の経過と、その出力の一部である。
type ActionLogRead struct {
	// Generation は、接続先で操作を始めるたびに 1 つ増える。読む側は、自分が始めた操作の
	// 出力かどうかをこれで見分ける。
	Generation uint64
	Action     EngineAction
	Done       bool
	// Succeeded は、接続先の sshc が 0 で終わったかである。Done のときだけ意味がある。
	Succeeded bool
	// Failure は、接続先の sshc の終了コードではなく、それ以外の理由で終わったときの理由である。
	// 接続先に sshc が無い、時間内に終わらない、SSH の接続が切れた、などである。
	Failure error
	// Read は、読んだ範囲である。位置は操作ごとに 0 から数える。
	Read terminal.RingRead
	// Output は、Read の範囲を、端末の制御文字を除いた文にしたものである。
	Output string
}

// actionLog は、接続先ひとつについて、最後に始めた操作の経過と出力を持つ。出力は、
// 接続先の sshc が書くそばから Write で受け取る。
type actionLog struct {
	mutex      sync.Mutex
	generation uint64
	action     EngineAction
	running    bool
	succeeded  bool
	failure    error
	output     *terminal.Ring
}

// begin は、新しい操作を始め、その世代を返す。前の操作の出力は捨てる。走っている操作が
// あれば ErrActionRunning で断る。同じ接続先の操作を、確かめるのと始めるのを
// ひとつの mutex の中で行い、二つの要求が同時に始めることのないようにする。
func (l *actionLog) begin(action EngineAction) (uint64, error) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if l.running {
		return 0, ErrActionRunning
	}
	l.generation++
	l.action = action
	l.running = true
	l.succeeded = false
	l.failure = nil
	l.output = terminal.NewRing(actionOutputCapacity)
	return l.generation, nil
}

// Write は、走っている操作の出力を足す。
func (l *actionLog) Write(p []byte) (int, error) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.output.Write(p)
}

// finish は、走っている操作を終える。failure は、終了コード以外で終わったときの理由である。
func (l *actionLog) finish(succeeded bool, failure error) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.running = false
	l.succeeded = succeeded
	l.failure = failure
}

// read は、最後の操作の経過と、cursor から limit バイトまでの出力を返す。
func (l *actionLog) read(cursor uint64, limit int) (ActionLogRead, error) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if l.generation == 0 {
		return ActionLogRead{}, ErrNoAction
	}
	read, ok := l.output.ReadFrom(cursor, limit)
	if !ok {
		return ActionLogRead{}, ErrActionCursorAhead
	}
	return ActionLogRead{
		Generation: l.generation, Action: l.action, Done: !l.running,
		Succeeded: l.succeeded, Failure: l.failure,
		Read: read, Output: terminal.PlainTextFrom(read.Context, read.Emit),
	}, nil
}
