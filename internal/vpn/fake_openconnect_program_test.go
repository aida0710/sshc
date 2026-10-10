package vpn

import (
	"fmt"
	"os"
	"os/signal"
	"testing"
)

// fakeOpenConnectVariable は、検査の実行ファイルを偽の openconnect として起動するときに
// 付ける環境変数である。
const fakeOpenConnectVariable = "SSHC_VPN_FAKE_OPENCONNECT"

// runFakeOpenConnect は、装置の承認を待っている openconnect の代わりをする。SIGINT を
// 受けるまで待ち、受けたら装置からログオフしたことにして logged-off と書いて終わる。
//
// sh は背後で起動したコマンドに SIGINT を無視させ、uutils の timeout はその無視を下の
// コマンドへ引き継ぐ。openconnect は SIGINT の扱いを sigaction で自分で決めるので、それ
// でも受け取れる。sh の trap は起動したときに無視されていた合図を扱えないので、
// openconnect の代わりにならない。Go の signal.Notify は、無視されていた SIGINT も受け
// 取れるようにする。
func runFakeOpenConnect() int {
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	fmt.Println("openconnect-ready")
	<-interrupts
	fmt.Println("logged-off")
	return 1
}

// fakeOpenConnectProgram は、偽の openconnect の実行ファイルのパスと、それを起動する
// プロセスに渡す環境変数を返す。
func fakeOpenConnectProgram(t *testing.T) (path string, environment []string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return self, append(os.Environ(), fakeOpenConnectVariable+"=1")
}
