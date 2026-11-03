package mobile

import (
	"context"
	"crypto/rand"
	"log/slog"
	"net"
	"os"
	"path"
	"path/filepath"

	"sshc/internal/app"
	"sshc/internal/handoff"
	"sshc/internal/releasewatch"
	"sshc/internal/terminal"
	"sshc/internal/ui"
)

// mobileDefaultPort は、WebView の origin（http://127.0.0.1:<port>）を engine の起動を
// またいで保つための既定の port である。origin が変わると WebView は別のサイトとして
// localStorage と OPFS を空から始めるので、テーマなどの設定が起動のたびに既定値へ戻り、
// 途中で止まったダウンロードの一時データも同じ origin から片付けられなくなる。
// desktop の app.DefaultPort とは別の値にし、同じデバイスで動く desktop 版とは origin を
// 共有しない。埋まっていれば、engine が代わりの port を 1 度選んで保存する。
const mobileDefaultPort = 54448

// dependencyOptions は、モバイル環境用の依存を組み立てるのに要るものである。
type dependencyOptions struct {
	// ctx は engine の寿命である。リリースの確認はこれが取り消されると止まる。
	ctx context.Context
	// goos はテスト可能にするため引数で受ける。
	goos     string
	home     string
	cache    string
	logger   *slog.Logger
	announce func(app.Readiness) error
}

// newDependencies はモバイル環境用の依存を組み立てる。自己更新、ssh-keygen、
// ssh-agent など利用できない機能は nil とする。
func newDependencies(options dependencyOptions) (app.Dependencies, error) {
	assets, err := ui.FS()
	if err != nil {
		return app.Dependencies{}, err
	}
	return app.Dependencies{
		Random:      rand.Reader,
		Announce:    options.announce,
		Listen:      net.Listen,
		DefaultPort: mobileDefaultPort,
		UI:          assets,
		Logger:      options.logger,
		Home:        options.home,
		Owner:       handoff.OwnerEngine,
		PID:         os.Getpid(),
		// モバイルには ssh-keygen と ssh-agent がない。
		Toolchain: nil,
		KeyAgent:  nil,
		// モバイルアプリの SHELL 環境変数には依存しない。
		Lookup:  func(string) (string, bool) { return "", false },
		Environ: mobileEnvironment(options.goos, options.home, options.cache),
		// モバイルアプリはバイナリを置換できないので SelfUpdate は無い。新しいリリースは調べて、
		// 画面がリリースページへ案内する。
		Updates: releasewatch.NewForVersion(options.ctx, version),
		// SFTP のダウンロードはアプリのキャッシュで用意する。アプリのプロセスには
		// HOME が無く、既定の一時ディレクトリ（/data/local/tmp）には書き込めない。
		SFTPDownloadSpoolRoot: filepath.Join(options.cache, "sftp-spool"),
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
		"TERM="+terminal.DefaultTerminalType,
		// 対象端末内のパスなので、ビルドホスト依存の filepath ではなく path を使う。
		"TMPDIR="+path.Clean(cache),
	)
	return func() []string { return append([]string(nil), environ...) }
}
