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

// watchParent は、起こしてくれた親が居なくなったら stop を呼ぶ。
//
// **親が死んだ子は引き取られる。** それを見て自分で畳むのは、このアプリケー
// ションが「終了すれば全部止まる」と言えるための最後の網である。アプリが
// SIGKILL された日でも、エンジンだけが残ることはない。
//
// 見るのは「1 になったか」ではなく「**起動時と変わったか**」である。引き取り手
// が init とは限らない——Linux で `systemd --user` が child subreaper を立てて
// いれば、孤児の ppid はそちらの pid になり、1 にはならない。そこでこの網は
// 静かに効かなくなる。original から変わったことは、どちらの引き取り手でも
// 等しく真である。
func watchParent(ctx context.Context, parent func() int, original int, tick time.Duration, stop func()) {
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if parent() != original {
				stop()
				return
			}
		}
	}
}
