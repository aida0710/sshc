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

// newDependencies は、Android で成立する形の依存一式を組み立てる。
//
// cmd/sshc/engine.go の runEngineApp と同じ役目だが、落としているものが 4 つ
// ある: 所有権の監視（親が死ねば道連れなので監視する対象が無い）、シグナル
// （Android はプロセスにシグナルを送って落とさない）、終了コードへの写像
// （返すのは error である）、そして自己更新（バイナリを置き換える経路が無い）。
func newDependencies(
	home, cache string,
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
		Owner:    handoff.OwnerDesktop,
		PID:      os.Getpid(),
		// **どちらも nil が答えである。** ssh-keygen も ssh-agent も Android に
		// 居ないので、それを探す道具を持つこと自体が嘘になる。受け側は既に
		// nil を機能の不在として扱う——keys.CatalogueReader はハードウェア鍵を
		// 一覧に足さず、keys.Service は到達できるエージェントが無いと答える。
		Toolchain: nil,
		KeyAgent:  nil,
		// SHELL を読まない。Android でそれを設定した人は居ないので、
		// 読めば偶然の値が権威になる。
		Lookup:  func(string) (string, bool) { return "", false },
		Environ: androidEnvironment(home, cache),
		// **置き換えられない更新を提示しない。**
		Updates: nil,
	}, nil
}

// androidEnvironment は、埋め込みターミナルが継ぐ環境を固定で組み立てる。
//
// os.Environ を渡さないのは、Android アプリの環境に有用な PATH が無いためで
// ある。そのまま渡せば、/system/bin すら見えないシェルが開く。
//
// 呼ばれるたびに写しを返す。同じ slice を返すと、受け取った側が append した
// ものが次の呼び出しに見える——このアプリケーションが渡す環境は、渡した先の
// 事情で変わってはならない。
func androidEnvironment(home, cache string) func() []string {
	environ := []string{
		"HOME=" + home,
		"PATH=/system/bin:/system/xbin",
		"TERM=xterm-256color",
		// **path であって path/filepath ではない。** ここで組み立てているのは
		// Android の中の道であり、区切りは常に "/" である。filepath は
		// これをコンパイルしたホストの区切りを使うので、Windows から見ると
		// TMPDIR が \data\user\0\app\cache になる。
		"TMPDIR=" + path.Clean(cache),
	}
	return func() []string { return append([]string(nil), environ...) }
}
