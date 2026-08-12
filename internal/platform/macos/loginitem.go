//go:build darwin

package macos

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"sshc/internal/platform"
)

// LoginItemLabel は、このアプリケーションが登録する launchd のラベル。
const LoginItemLabel = "com.github.aida0710.sshc"

// ErrLoginItemPathNotAbsolute は、launchd が PATH 経由で探さなければならない
// プログラム、つまり他人が供給しうるプログラムの登録を拒否する。
var ErrLoginItemPathNotAbsolute = errors.New("login item program path must be absolute")

// LoginItem は「ログイン時に sshc を起動する」を切り替える。
//
// ユーザーが求めない限りオフである。保存済みのあらゆる秘密の鍵を握る
// バックグラウンドプロセスは、他人に代わって勝手に用意してよいものではないし、
// これがなくてもアプリケーションは十分に使える。何も動いていなければ
// `sshc <alias>` は素の ssh にフォールバックする。
//
// エージェントは -open=false で起動されるので、ログイン時にブラウザが開くことは
// ない。標準出力を意図的にどこにもリダイレクトしていないのは、それが有効な
// ブートストラップトークン付きの URL を運ぶからであり、ログファイルはそれを
// 置く場所ではない。誰かが見たくなったときは `sshc open` が新しいものを発行する。
type LoginItem struct {
	// Runner は launchctl を実行する。テストがエージェントを読み込まないよう注入する。
	Runner platform.OutputRunner
	// Home はユーザーのホームディレクトリ。LaunchAgents が置かれる場所である。
	Home string
	// Launchctl はそのプログラム。argv を見たいテストのためにある。
	Launchctl string
}

func (l LoginItem) plistPath() string {
	return filepath.Join(l.Home, "Library", "LaunchAgents", LoginItemLabel+".plist")
}

// Registered はplistの有無と、それを確かめられなかった状態を区別する。Web表示は
// boolだけを必要とするが、install/uninstallは不明な状態を「無効」として成功させない。
func (l LoginItem) Registered() (bool, error) {
	_, err := os.Stat(l.plistPath())
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// Enabled はWeb設定用のboolを返す。保守コマンドはRegisteredを直接使い、判定時の
// エラーを失わない。
func (l LoginItem) Enabled() bool {
	enabled, err := l.Registered()
	return err == nil && enabled
}

// Enable はエージェントを書き出し、launchd に取り込むよう求める。
func (l LoginItem) Enable(ctx context.Context, program string) error {
	if !filepath.IsAbs(program) {
		return ErrLoginItemPathNotAbsolute
	}
	path := l.plistPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(agentPlist(program)), 0o600); err != nil {
		return err
	}
	// 先に bootout するのは、以前のものを読み込んだままにせず、プログラムパスの
	// 変更を拾わせるためである。
	if err := l.bootout(ctx); err != nil {
		return err
	}
	return l.launchctl(ctx, "bootstrap", l.domain(), path)
}

// Disable はエージェントを止め、ファイルを取り除く。
func (l LoginItem) Disable(ctx context.Context) error {
	if err := l.bootout(ctx); err != nil {
		return err
	}
	if err := os.Remove(l.plistPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (l LoginItem) bootout(ctx context.Context) error {
	output, err := l.runLaunchctl(ctx, "bootout", l.domain()+"/"+LoginItemLabel)
	if err != nil {
		return err
	}
	// launchctl 3 はNo such process、つまりplistはあっても今は未ロードである。
	// Disableには既に望んだ状態であり、Enableもそのままbootstrapへ進めてよい。
	if output.ExitCode == 0 || output.ExitCode == 3 {
		return nil
	}
	return fmt.Errorf("launchctl bootout exited with status %d", output.ExitCode)
}

func (l LoginItem) domain() string {
	return "gui/" + strconv.Itoa(os.Getuid())
}

func (l LoginItem) launchctl(ctx context.Context, arguments ...string) error {
	output, err := l.runLaunchctl(ctx, arguments...)
	if err != nil {
		return err
	}
	if output.ExitCode != 0 {
		return fmt.Errorf("launchctl %s exited with status %d", arguments[0], output.ExitCode)
	}
	return nil
}

func (l LoginItem) runLaunchctl(ctx context.Context, arguments ...string) (platform.Output, error) {
	program := l.Launchctl
	if program == "" {
		program = "/bin/launchctl"
	}
	if l.Runner == nil {
		return platform.Output{}, errors.New("no runner to start launchctl with")
	}
	return l.Runner.RunOutput(ctx, platform.Command{Path: program, Arguments: arguments})
}

// agentPlist は launchd が読むプロパティリスト。
//
// プログラムパスは生のまま連結せず XML エスケープする。これは os.Executable から
// 来るものでユーザー入力ではないが、アンパサンドを含むパスがあると、launchd が
// 解析を拒否するファイルができてしまうからだ。
func agentPlist(program string) string {
	escape := func(value string) string {
		replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
		return replacer.Replace(value)
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>-open=false</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>ProcessType</key>
	<string>Background</string>
</dict>
</plist>
`, LoginItemLabel, escape(program))
}
