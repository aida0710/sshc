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

// newDependencies はモバイル環境用の依存を組み立てる。自己更新、ssh-keygen、
// ssh-agent など利用できない機能は nil とする。goos はテスト可能にするため引数で受ける。
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
		// モバイルには ssh-keygen と ssh-agent がない。
		Toolchain: nil,
		KeyAgent:  nil,
		// モバイルアプリの SHELL 環境変数には依存しない。
		Lookup:  func(string) (string, bool) { return "", false },
		Environ: mobileEnvironment(goos, home, cache),
		// モバイルアプリからバイナリを置換できないため、自己更新を無効にする。
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
// 呼び出し側の変更が残らないよう、毎回 slice のコピーを返す。
func mobileEnvironment(goos, home, cache string) func() []string {
	environ := []string{"HOME=" + home}
	// Android はシェルを起動できるため system PATH を設定する。iOS では省略する。
	if goos == "android" {
		environ = append(environ, "PATH=/system/bin:/system/xbin")
	}
	environ = append(environ,
		"TERM=xterm-256color",
		// 対象端末内のパスなので、ビルドホスト依存の filepath ではなく path を使う。
		"TMPDIR="+path.Clean(cache),
	)
	return func() []string { return append([]string(nil), environ...) }
}
