//go:build linux

package linux

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sshc/internal/platform"
)

// UnitName は、このアプリケーションが登録する systemd user unit の名前。
const UnitName = "sshc.service"

// ErrLoginItemPathNotAbsolute は、systemd が PATH 経由で探さなければならない
// プログラム、つまり他人が供給しうるプログラムの登録を拒否する。
var ErrLoginItemPathNotAbsolute = errors.New("login item program path must be absolute")

// LoginItem は「ログイン時に sshc を起動する」を切り替える。
//
// ユーザーが求めない限りオフである。保存済みのあらゆる秘密の鍵を握る
// バックグラウンドプロセスは、他人に代わって勝手に用意してよいものではないし、
// これがなくてもアプリケーションは十分に使える。何も動いていなければ
// `sshc <alias>` は素の ssh にフォールバックする。
//
// エージェントは -open=false で起動し、標準出力をどこにもリダイレクトしない。
// あの出力には有効な bootstrap トークン付きの URL が乗るので、journald はその
// 置き場所ではない。誰かが見たくなったときは `sshc open` が新しいものを発行する。
type LoginItem struct {
	// Runner は systemctl を実行する。テストが unit を読み込まないよう注入する。
	Runner platform.OutputRunner
	// Home はユーザーのホームディレクトリ。user unit が置かれる場所である。
	Home string
	// Systemctl はそのプログラム。argv を見たいテストのためにある。
	Systemctl string
}

func (l LoginItem) unitPath() string {
	return filepath.Join(l.Home, ".config", "systemd", "user", UnitName)
}

func (l LoginItem) systemctl() string {
	if l.Systemctl == "" {
		return "/usr/bin/systemctl"
	}
	return l.Systemctl
}

// Enabled は、unit が登録されているかを報告する。
//
// error を返さないのは、呼び出し側のインターフェースがそう決めているからである。
// 読めないことと登録されていないことは、この設定を表示する画面にとって同じ答えで
// ある。
func (l LoginItem) Enabled() bool {
	_, err := os.Stat(l.unitPath())
	return err == nil
}

// Enable は unit を書き出し、systemd に取り込ませる。
func (l LoginItem) Enable(ctx context.Context, program string) error {
	executablePath := program
	if !filepath.IsAbs(executablePath) || strings.ContainsAny(executablePath, "\n\r") {
		// 改行が入れば unit ファイルが壊れる。絶対パスでなければ systemd が
		// PATH 経由で探すことになり、それは他人が供給しうるプログラムである。
		return fmt.Errorf("%s: %w", executablePath, ErrLoginItemPathNotAbsolute)
	}
	if err := os.MkdirAll(filepath.Dir(l.unitPath()), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(l.unitPath(), []byte(unitFor(executablePath)), 0o600); err != nil {
		return err
	}
	// 先に daemon-reload するのは、以前のものを読み込んだままにせず、
	// プログラムパスの変更を拾わせるためである。
	if _, err := l.run(ctx, "--user", "daemon-reload"); err != nil {
		return err
	}
	_, err := l.run(ctx, "--user", "enable", "--now", UnitName)
	return err
}

// Disable は unit を止め、ファイルを取り除く。
func (l LoginItem) Disable(ctx context.Context) error {
	if _, err := l.run(ctx, "--user", "disable", "--now", UnitName); err != nil {
		return err
	}
	if err := os.Remove(l.unitPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_, err := l.run(ctx, "--user", "daemon-reload")
	return err
}

func (l LoginItem) run(ctx context.Context, arguments ...string) (platform.Output, error) {
	return l.Runner.RunOutput(ctx, platform.Command{
		Path:      l.systemctl(),
		Arguments: arguments,
	})
}

// unitFor は systemd が読む unit ファイル。
//
// 標準出力を null へ送るのは、エージェントが表示する URL が有効な bootstrap
// トークンを運ぶからである。journald に残せば、それを読める者が入口を得る。
func unitFor(executablePath string) string {
	return "[Unit]\n" +
		"Description=sshc\n" +
		"\n" +
		"[Service]\n" +
		"Type=simple\n" +
		"ExecStart=" + executablePath + " -open=false\n" +
		"StandardOutput=null\n" +
		"StandardError=journal\n" +
		"Restart=on-failure\n" +
		"\n" +
		"[Install]\n" +
		"WantedBy=default.target\n"
}
