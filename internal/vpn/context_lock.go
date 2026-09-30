package vpn

import (
	"context"
	"sync"
	"sync/atomic"
)

// contextLock は、待っているあいだも ctx の取り消しに従う鍵である。
//
// 経路の起動は、イメージの作成と承認待ちで分単位になる。sync.Mutex で待つと、停止や
// 削除を頼んだ側が諦めても（CLI の Ctrl-C など）、起動が終わるまで戻れない。ゼロ値の
// まま使える。
type contextLock struct {
	once sync.Once
	held chan struct{}
	// waiting は、鍵を待っている呼び出しの数である。検査は、呼び出しが鍵を待ち始めた
	// ことをこれで確かめてから次へ進む。時間で待つと、遅いマシンでは待ち始める前に進む。
	waiting atomic.Int32
}

func (lock *contextLock) channel() chan struct{} {
	lock.once.Do(func() { lock.held = make(chan struct{}, 1) })
	return lock.held
}

// lock は、鍵を取る。取る前に ctx が終われば、取らずに ctx の理由を返す。
func (lock *contextLock) lock(ctx context.Context) error {
	// 鍵が空いていても、もう諦めた呼び出し側には取らせない。
	if err := ctx.Err(); err != nil {
		return err
	}
	lock.waiting.Add(1)
	defer lock.waiting.Add(-1)
	select {
	case lock.channel() <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// unlock は、lock で取った鍵を返す。
func (lock *contextLock) unlock() {
	<-lock.channel()
}
