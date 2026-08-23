//go:build unix

package integration

import (
	"syscall"
	"testing"
	"time"
)

// **止め方で終わり方が変わる。** Ctrl-C は人が止めたので 130、SIGTERM は
// 監督者が止めたので 0 である。この違いは、`sshc engine` を supervisor の
// 下で走らせた人にとって意味を持つ——130 で終わるものを「異常終了」と読んで
// 再起動し続ける監督者は珍しくない。
//
// **同じプロセスの中では確かめられない。** 信号はプロセスに届くものであり、
// 関数を呼び合っても届かない。
func TestHowTheEngineIsStoppedDecidesHowItEnds(t *testing.T) {
	for _, test := range []struct {
		name   string
		signal syscall.Signal
		want   int
	}{
		{name: "Ctrl-C is a person stopping it", signal: syscall.SIGINT, want: 130},
		{name: "SIGTERM is a supervisor stopping it", signal: syscall.SIGTERM, want: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := isolatedHome(t)
			engine := start(t, home, "engine")
			waitForFile(t, handoffPath(home), 30*time.Second, engine)

			if err := engine.Command.Process.Signal(test.signal); err != nil {
				t.Fatal(err)
			}

			if code := engine.wait(t, 30*time.Second); code != test.want {
				t.Errorf("exit = %d, want %d\n%s", code, test.want, engine.Stderr.String())
			}
			// **畳んでから終わる。** 次の owner のために席が空いていることが、
			// 片付けが最後まで走った証拠である。
			takeOverAsHeadless(t, home)
		})
	}
}
