package mobile

import (
	"crypto/rand"
	"log/slog"
	"net"
	"os"
	"path"

	"sshc/internal/app"
	"sshc/internal/handoff"
	"sshc/internal/ui"
)

// newDependencies は、モバイルで成立する形の依存一式を組み立てる。
//
// cmd/sshc/engine.go の runEngineApp と同じ役目だが、こちらが引き受けないものが
// ある: 所有権の監視（親が死ねば道連れなので監視する対象が無い）、シグナル
// （モバイルの OS はプロセスにシグナルを送って落とさない）、終了コードへの写像
// （返すのは error である）。
//
// app.Dependencies の側で意図的に空のままにするのは、自己更新（バイナリを置き
// 換える経路が無い）と、ssh-keygen・ssh-agent（どちらの OS にも居ない）である。
//
// **数を数えた文章をここに書かない。** 以前は「落としているものが 4 つある」と
// 書いてあり、実際には 6 つだった。どの項目が空でよいのかは dependencies_test.go
// が一つずつ表明しており、app.Dependencies に項目が増えれば、そこが赤くなって
// 選択を促す。散文はその数を追いかけられない。
// **goos を引数で受ける。** ここで runtime.GOOS を読むと、走っているマシンでしか
// 通らない表明になる——Linux 上のテストは Android の姿を一度も確かめられない。
func newDependencies(
	goos, home, cache string,
	logger *slog.Logger,
	announce func(app.Readiness) error,
) (app.Dependencies, error) {
	assets, err := ui.FS()
	if err != nil {
		return app.Dependencies{}, err
	}
	return app.Dependencies{
		Random:   rand.Reader,
		Announce: announce,
		Listen:   net.Listen,
		UI:       assets,
		Logger:   logger,
		Home:     home,
		Owner:    handoff.OwnerEngine,
		PID:      os.Getpid(),
		// **どちらも nil が答えである。** ssh-keygen も ssh-agent もモバイルに
		// 居ないので、それを探す道具を持つこと自体が嘘になる。受け側は既に
		// nil を機能の不在として扱う——keys.CatalogueReader はハードウェア鍵を
		// 一覧に足さず、keys.Service は到達できるエージェントが無いと答える。
		Toolchain: nil,
		KeyAgent:  nil,
		// SHELL を読まない。モバイルでそれを設定した人は居ないので、
		// 読めば偶然の値が権威になる。
		Lookup:  func(string) (string, bool) { return "", false },
		Environ: mobileEnvironment(goos, home, cache),
		// **置き換えられない更新を提示しない。**
		Updates: nil,
	}, nil
}

// mobileEnvironment は、埋め込みターミナルが継ぐ環境を固定で組み立てる。
//
// os.Environ を渡さないのは、モバイルアプリの環境に有用な PATH が無いためで
// ある。そのまま渡せば、/system/bin すら見えないシェルが開く。
//
// goos を引数で受けるのは、この表がテストできるようにするためである。
// runtime.GOOS をここで読むと、走っているマシンでしか通らない表明になる。
//
// 呼ばれるたびに写しを返す。同じ slice を返すと、受け取った側が append した
// ものが次の呼び出しに見える——このアプリケーションが渡す環境は、渡した先の
// 事情で変わってはならない。
func mobileEnvironment(goos, home, cache string) func() []string {
	environ := []string{"HOME=" + home}
	// **PATH を置くのは、そこを歩ける端末だけである。**
	//
	// Android はシェルを起こせるので、/system/bin が見えなければならない。
	// iOS はプロセスを起こせないので、PATH は誰も読まない——置けば「歩ける道が
	// ある」と言っているのと同じで、嘘になる。
	if goos == "android" {
		environ = append(environ, "PATH=/system/bin:/system/xbin")
	}
	environ = append(environ,
		"TERM=xterm-256color",
		// **path であって path/filepath ではない。** ここで組み立てているのは
		// その端末の中の道であり、区切りは常に "/" である。filepath は
		// これをコンパイルしたホストの区切りを使うので、Windows から見ると
		// TMPDIR が \data\user\0\app\cache になる。
		"TMPDIR="+path.Clean(cache),
	)
	return func() []string { return append([]string(nil), environ...) }
}
