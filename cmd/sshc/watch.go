package main

import (
	"context"
	"time"
)

// parentTick は、親を見に行く間隔である。
//
// **これは異常終了のための網であり、通常の終了経路ではない。** 普通に終わる
// ときは親が kill するので、ここが気づくまでの 1 秒は誰も待たない。
const parentTick = time.Second

// watchParent は、親が居なくなったら stop を呼ぶ。
//
// **親が死んだ子は init に引き取られる。** それを見て自分で畳むのは、
// このアプリケーションが「終了すれば全部止まる」と言えるための最後の網である。
// アプリが SIGKILL された日でも、エンジンだけが残ることはない。
func watchParent(ctx context.Context, parent func() int, tick time.Duration, stop func()) {
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if parent() == 1 {
				stop()
				return
			}
		}
	}
}
