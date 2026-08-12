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

// DefaultSystemctl は Systemctl が空のときに使う絶対パスである。組み立て側が
// systemd の有無を probe するときも同じ定数を読むので、probe と既定値が
// 別々に書かれてずれることはない。
const DefaultSystemctl = "/usr/bin/systemctl"

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
		return DefaultSystemctl
	}
	return l.Systemctl
}

// Registered はunitの有無と、それを確かめられなかった状態を区別する。Webの表示は
// boolだけを必要とするが、install/uninstallは「読めない」を「無い」とみなして
// 壊れた常駐設定を残したまま成功してはならない。
func (l LoginItem) Registered() (bool, error) {
	_, err := os.Stat(l.unitPath())
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// Enabled は、unit が登録されているかをWeb設定用のboolで報告する。
// 読めない場合はスイッチをオンとは表示しない。保守コマンドはRegisteredを直接使い、
// そのエラーを失わない。
func (l LoginItem) Enabled() bool {
	enabled, err := l.Registered()
	return err == nil && enabled
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
	if _, err := l.run(ctx, "--user", "enable", UnitName); err != nil {
		return err
	}
	// enable --now は未起動のunitを始めるが、既に動いているunitを再起動しない。
	// ExecStartのパスを書き換えた更新では、明示的なrestartだけが新しい実行ファイルへ
	// プロセスを付け替える。
	_, err := l.run(ctx, "--user", "restart", UnitName)
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
	// % は unit ファイルの中で specifier の接頭辞（%h、%i、%% など）なので、
	// パスにリテラルの % が含まれていれば二重にする。ガードでは拒否できない。
	// % はファイル名として合法な文字だからである。
	escapedPath := strings.ReplaceAll(executablePath, "%", "%%")
	return "[Unit]\n" +
		"Description=sshc\n" +
		"\n" +
		"[Service]\n" +
		"Type=simple\n" +
		"ExecStart=" + escapedPath + " -open=false\n" +
		"StandardOutput=null\n" +
		"StandardError=journal\n" +
		"Restart=on-failure\n" +
		"\n" +
		"[Install]\n" +
		"WantedBy=default.target\n"
}
