package remoteengine

import "testing"

// 中継を閉じたら、戻った時点でポートは空いている。起動し直したエンジンは同じポートで待ち受け直す。
// http.Server.Close は、Serve が受け取り済みの待ち受けしか閉じない。Serve の goroutine が走る前に
// 閉じると、待ち受けはあとで Serve が閉じるまで残る（macOS の CI で起きた）。
func TestAClosedRelayHasLetGoOfItsPortEvenIfItsServerHadNotStarted(t *testing.T) {
	for range 100 {
		listener, port, err := listenLoopback(0)
		if err != nil {
			t.Fatal(err)
		}
		relay := newRelay(newHostLink(nil, 0))
		relay.serve(listener, port)
		relay.close()

		again, _, err := listenLoopback(port)
		if err != nil {
			t.Fatalf("port %d is still held after the relay closed: %v", port, err)
		}
		_ = again.Close()
	}
}
