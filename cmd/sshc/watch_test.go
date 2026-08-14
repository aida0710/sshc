package main

import (
	"context"
	"testing"
	"time"
)

// **親が死んだ子は引き取られる。** それがこの見張りの唯一の手掛かりである。
// 通常の終了では親が kill するので、ここは異常終了のための最後の網である。
//
// 引き取り手は init（1）とは限らない。macOS では 1 だが、Linux で
// `systemd --user` が child subreaper を立てていれば、孤児の ppid はそちらの
// pid になる。**どちらでも「起動時の親から変わった」ことだけは真である。**
func TestWatchParentStopsWhenTheParentIsGone(t *testing.T) {
	for _, test := range []struct {
		name     string
		reparent int
	}{
		{name: "init adopts the orphan", reparent: 1},
		{name: "a subreaper adopts the orphan", reparent: 9182},
	} {
		t.Run(test.name, func(t *testing.T) {
			const original = 4242
			readings := []int{original, original, test.reparent}
			index := 0
			parent := func() int {
				reading := readings[index]
				if index < len(readings)-1 {
					index++
				}
				return reading
			}

			stopped := make(chan struct{})
			go watchParent(context.Background(), parent, original, time.Millisecond, func() { close(stopped) })

			select {
			case <-stopped:
			case <-time.After(2 * time.Second):
				t.Fatal("the watch never noticed the parent was gone")
			}
		})
	}
}

// 見張りは、止めろと言われたら止める。親が居るあいだは何もしない。
func TestWatchParentLetsGoWhenTheContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		watchParent(ctx, func() int { return 4242 }, 4242, time.Millisecond, func() {
			t.Error("the watch stopped a process whose parent was alive")
		})
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the watch ignored the context")
	}
}
