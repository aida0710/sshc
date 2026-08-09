//go:build linux

package linux_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshc/internal/platform"
	"sshc/internal/platform/linux"
)

// 本物の systemd を読み込むテストはない。ランナーは systemctl が何を求められた
// はずかを記録するだけで、何もしない。
type unitRunner struct{ commands []platform.Command }

func (runner *unitRunner) RunOutput(_ context.Context, command platform.Command) (platform.Output, error) {
	runner.commands = append(runner.commands, command)
	return platform.Output{}, nil
}

// ログイン時には何も開かず、標準出力をどこにも送らない。エージェントが表示する
// URL は有効な bootstrap トークンを運び、journald はその置き場所ではない。
func TestEnableWritesAUnitThatOpensNothingAndLogsNothing(t *testing.T) {
	home := t.TempDir()
	runner := &unitRunner{}
	item := linux.LoginItem{Runner: runner, Home: home, Systemctl: "/usr/bin/systemctl"}

	if err := item.Enable(context.Background(), "/home/u/.local/bin/sshc"); err != nil {
		t.Fatal(err)
	}
	unit, err := os.ReadFile(filepath.Join(home, ".config", "systemd", "user", "sshc.service"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(unit)
	if !strings.Contains(text, "ExecStart=/home/u/.local/bin/sshc -open=false") {
		t.Errorf("unit does not start the agent with the browser off:\n%s", text)
	}
	if !strings.Contains(text, "StandardOutput=null") {
		t.Errorf("unit sends standard output somewhere:\n%s", text)
	}
}

// 絶対パスでなければ登録しない。systemd に PATH 経由で探させるプログラムは、
// 他人が供給しうるプログラムである。
func TestEnableRefusesARelativeProgram(t *testing.T) {
	runner := &unitRunner{}
	item := linux.LoginItem{Runner: runner, Home: t.TempDir(), Systemctl: "/usr/bin/systemctl"}

	if err := item.Enable(context.Background(), "sshc"); err == nil {
		t.Fatal("a relative program was registered")
	}
	if len(runner.commands) != 0 {
		t.Error("systemctl was run anyway")
	}
}

// 改行が入ったパスも拒否する。改行は ExecStart= 行を終わらせ、その後に続くものを
// unit ディレクティブとして書き込ませてしまうからである。
func TestEnableRefusesAProgramWithANewline(t *testing.T) {
	home := t.TempDir()
	runner := &unitRunner{}
	item := linux.LoginItem{Runner: runner, Home: home, Systemctl: "/usr/bin/systemctl"}

	if err := item.Enable(context.Background(), "/usr/local/bin/sshc\nExecStart=/tmp/evil"); err == nil {
		t.Fatal("a program path containing a newline was registered")
	}
	if len(runner.commands) != 0 {
		t.Error("systemctl was run anyway")
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "systemd", "user", "sshc.service")); !os.IsNotExist(err) {
		t.Errorf("unit file was created despite the rejection: %v", err)
	}
}

// % は unit ファイルの中で specifier の接頭辞なので、パスにリテラルの % があれば
// 二重にして書かなければならない。そうしなければ systemd がそれを specifier として
// 展開してしまう。
func TestEnableEscapesPercentInTheProgramPath(t *testing.T) {
	home := t.TempDir()
	item := linux.LoginItem{Runner: &unitRunner{}, Home: home, Systemctl: "/usr/bin/systemctl"}

	if err := item.Enable(context.Background(), "/opt/50%/sshc"); err != nil {
		t.Fatal(err)
	}
	unit, err := os.ReadFile(filepath.Join(home, ".config", "systemd", "user", "sshc.service"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(unit), "ExecStart=/opt/50%%/sshc -open=false") {
		t.Errorf("unit does not double the percent sign:\n%s", unit)
	}
}

// 二度無効にすることは、呼び出し側が求めた状態である。
func TestDisableTwiceIsTheStateTheCallerAskedFor(t *testing.T) {
	home := t.TempDir()
	item := linux.LoginItem{Runner: &unitRunner{}, Home: home, Systemctl: "/usr/bin/systemctl"}

	if err := item.Disable(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := item.Disable(context.Background()); err != nil {
		t.Fatal(err)
	}
}
