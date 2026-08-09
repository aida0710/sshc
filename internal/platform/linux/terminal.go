//go:build linux

package linux

import (
	"context"
	"errors"
	"path/filepath"

	"sshc/internal/platform"
)

// ErrHelperPathNotAbsolute は、このアプリケーションが PATH 経由で探さなければ
// ならないヘルパー、つまり他人が供給しうるヘルパーを拒否する。
var ErrHelperPathNotAbsolute = errors.New("askpass helper path must be absolute")

// sshProgram は、パスワードを使わない接続が起動するプログラムの絶対パス。
const sshProgram = "/usr/bin/ssh"

// Terminal は、ユーザーが指定したプログラムで対話的な SSH セッションを開く。
//
// 端末の表は持たない。macOS では CLI を持たない端末が Terminal.app と iTerm2 の
// 二つで打ち止めなので、profile の表が意味を持つ。Linux では端末が乱立していて、
// 実行するコマンドの渡し方も端末ごとに違う（-e、--、-x、ひとつに繋いだ文字列を
// 欲しがるものもあれば、語のリストを欲しがるものもある）。表を用意すれば、そこに
// 無い端末を使う人には効かず、そこにある端末でも規約を取り違えれば黙って壊れる。
// だからユーザーがコマンドそのものを書き、こちらは推測しない。
type Terminal struct {
	runner platform.OutputRunner
}

// NewTerminal は Linux の端末ランチャーを返す。
func NewTerminal(runner platform.OutputRunner) Terminal {
	return Terminal{runner: runner}
}

// Applications は空を返す。custom の開く先は選択そのものが運び、この在庫が
// 足すものは何もない。
func (t Terminal) Applications() []platform.Application {
	return make([]platform.Application, 0)
}

// Terminals は custom の 1 件だけを返す。定義済みの端末を持たないので、
// これが選択肢の全体である。
func (t Terminal) Terminals() []platform.TerminalAvailability {
	return []platform.TerminalAvailability{{ID: platform.TerminalCustom, Installed: true}}
}

// Launch は既定を持たない。何で開くかを利用者が指定していないので、推測して
// 開く先がない。
func (t Terminal) Launch(_ context.Context, _ string) error {
	return platform.ErrTerminalApplication
}

// LaunchWithPassword も同じ理由で既定を持たない。
func (t Terminal) LaunchWithPassword(_ context.Context, _, _, _, _ string) error {
	return platform.ErrTerminalApplication
}

// LaunchIn は、choice の指すプログラムを choice.Arguments に続けて
// `ssh -- alias` で起動する。
func (t Terminal) LaunchIn(ctx context.Context, choice platform.TerminalChoice, alias string) error {
	if err := t.validate(choice, alias); err != nil {
		return err
	}
	return t.run(ctx, choice, []string{sshProgram, "--", alias})
}

// LaunchWithPasswordIn は、choice の指すプログラムを choice.Arguments に続けて
// `helperPath alias` で起動する。
//
// endpoint と token はこの関数のどこにも現れない。使わないことこそが要点である。
// ウィンドウが実行するのは helperPath（このバイナリ自身）と alias だけで、
// トークンはそのプロセスが起動後に handoff から自分で取りに行く。コマンドラインに
// 載せれば、この利用者として動くすべてのプロセスに ps 経由で見えてしまう。
func (t Terminal) LaunchWithPasswordIn(
	ctx context.Context, choice platform.TerminalChoice, alias, helperPath, endpoint, token string,
) error {
	if err := t.validate(choice, alias); err != nil {
		return err
	}
	// helperPath が絶対でなければ systemd と同じ理由で拒否する。他人が供給しうる
	// プログラムを PATH 経由で探させてはならない。
	if !filepath.IsAbs(helperPath) {
		return ErrHelperPathNotAbsolute
	}
	return t.run(ctx, choice, []string{helperPath, alias})
}

// validate は、両方の経路が起動前に必ず通る検査である。custom しか差し出して
// いないので、他の ID をここで拒む。
func (t Terminal) validate(choice platform.TerminalChoice, alias string) error {
	if err := platform.ValidateTerminalChoice(choice); err != nil {
		return err
	}
	if err := platform.ValidateAlias(alias); err != nil {
		return err
	}
	if choice.ID != platform.TerminalCustom {
		return platform.ErrTerminalApplication
	}
	return nil
}

// run は、検査を通った choice と program から argv を組み立てて実行する。
//
// choice.Arguments には触れず、新しいスライスへコピーしてから追記する。呼び出し
// 側が同じ TerminalChoice を後で使い回すことがあるので、この関数がその値を
// 書き換えてはならない。
func (t Terminal) run(ctx context.Context, choice platform.TerminalChoice, program []string) error {
	arguments := make([]string, 0, len(choice.Arguments)+len(program))
	arguments = append(arguments, choice.Arguments...)
	arguments = append(arguments, program...)
	_, err := t.runner.RunOutput(ctx, platform.Command{Path: choice.Application, Arguments: arguments})
	return err
}
