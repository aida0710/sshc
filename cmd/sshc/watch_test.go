package main

import (
	"context"
	"testing"
	"time"
)

// **親が死んだ子は init に引き取られる。** それがこの見張りの唯一の手掛かりで
// ある。通常の終了では親が kill するので、ここは異常終了のための最後の網である。
func TestWatchParentStopsWhenTheParentIsGone(t *testing.T) {
	readings := []int{4242, 4242, 1}
	index := 0
	parent := func() int {
		reading := readings[index]
		if index < len(readings)-1 {
			index++
		}
		return reading
	}

	stopped := make(chan struct{})
	go watchParent(context.Background(), parent, time.Millisecond, func() { close(stopped) })

	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("the watch never noticed the parent was gone")
	}
}

// 見張りは、止めろと言われたら止める。親が居るあいだは何もしない。
func TestWatchParentLetsGoWhenTheContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		watchParent(ctx, func() int { return 4242 }, time.Millisecond, func() {
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
