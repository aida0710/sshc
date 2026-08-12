//go:build linux

package linux_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"sshc/internal/platform"
	"sshc/internal/platform/linux"
)

// 本物の systemd を読み込むテストはない。ランナーは systemctl が何を求められた
// はずかを記録するだけで、何もしない。
type unitRunner struct {
	commands []platform.Command
	outputs  []platform.Output
	errors   []error
}

func (runner *unitRunner) RunOutput(_ context.Context, command platform.Command) (platform.Output, error) {
	runner.commands = append(runner.commands, command)
	index := len(runner.commands) - 1
	var output platform.Output
	var err error
	if index < len(runner.outputs) {
		output = runner.outputs[index]
	}
	if index < len(runner.errors) {
		err = runner.errors[index]
	}
	return output, err
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

// 上の五つはすべて Systemctl を明示していたため、それが空のときに本番が
// 実際に取る DefaultSystemctl へのフォールバック（loginitem.go の
// systemctl メソッド）は、一度も実行されずに済んでいた。ここでは Systemctl
// を渡さず、記録された argv そのものを順序どおりに検査する。
func TestEnableFallsBackToDefaultSystemctlWhenNoneIsGiven(t *testing.T) {
	home := t.TempDir()
	runner := &unitRunner{}
	item := linux.LoginItem{Runner: runner, Home: home}

	if err := item.Enable(context.Background(), "/home/u/.local/bin/sshc"); err != nil {
		t.Fatal(err)
	}
	want := []platform.Command{
		{Path: linux.DefaultSystemctl, Arguments: []string{"--user", "daemon-reload"}},
		{Path: linux.DefaultSystemctl, Arguments: []string{"--user", "enable", linux.UnitName}},
		{Path: linux.DefaultSystemctl, Arguments: []string{"--user", "restart", linux.UnitName}},
	}
	if len(runner.commands) != len(want) {
		t.Fatalf("systemctl calls = %#v, want %#v", runner.commands, want)
	}
	for i, got := range runner.commands {
		if got.Path != want[i].Path || !slices.Equal(got.Arguments, want[i].Arguments) {
			t.Errorf("call %d = %#v, want %#v", i, got, want[i])
		}
	}
}

// Enabledのboolだけでは「無い」と「読めない」を区別できないが、保守コマンドが
// 後者を無視すると、取り残されたunitを更新も解除もできないまま成功を報告してしまう。
func TestRegisteredDistinguishesAbsentPresentAndUnreadableUnitState(t *testing.T) {
	t.Run("absent", func(t *testing.T) {
		item := linux.LoginItem{Home: t.TempDir()}
		registered, err := item.Registered()
		if err != nil || registered {
			t.Fatalf("Registered = %v, %v; want false, nil", registered, err)
		}
	})

	t.Run("present", func(t *testing.T) {
		home := t.TempDir()
		path := filepath.Join(home, ".config", "systemd", "user", linux.UnitName)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("[Service]\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		item := linux.LoginItem{Home: home}
		registered, err := item.Registered()
		if err != nil || !registered {
			t.Fatalf("Registered = %v, %v; want true, nil", registered, err)
		}
	})

	t.Run("unknown", func(t *testing.T) {
		home := t.TempDir()
		// .configがdirectoryでなければ、その下のunitをstatした結果は「存在しない」
		// ではなくENOTDIRである。保守操作はこの不明な状態をno-opにしてはならない。
		if err := os.WriteFile(filepath.Join(home, ".config"), []byte("not a directory"), 0o600); err != nil {
			t.Fatal(err)
		}
		item := linux.LoginItem{Home: home}
		registered, err := item.Registered()
		if err == nil || registered {
			t.Fatalf("Registered = %v, %v; want false and an error", registered, err)
		}
	})
}

// 二度無効にすることは、呼び出し側が求めた状態である。
func TestDisableTwiceIsTheStateTheCallerAskedFor(t *testing.T) {
	home := t.TempDir()
	runner := &unitRunner{}
	item := linux.LoginItem{Runner: runner, Home: home, Systemctl: "/usr/bin/systemctl"}

	if err := item.Disable(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := item.Disable(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("an absent unit still ran systemctl: %#v", runner.commands)
	}
}

func TestLoginItemReportsNonZeroSystemctlExits(t *testing.T) {
	t.Run("enable stops at daemon reload", func(t *testing.T) {
		runner := &unitRunner{outputs: []platform.Output{{ExitCode: 1}}}
		item := linux.LoginItem{Runner: runner, Home: t.TempDir(), Systemctl: "/usr/bin/systemctl"}
		if err := item.Enable(context.Background(), "/home/u/.local/bin/sshc"); err == nil {
			t.Fatal("daemon-reload exit 1 was reported as success")
		}
		if len(runner.commands) != 1 {
			t.Fatalf("commands after failure = %#v", runner.commands)
		}
	})

	t.Run("disable keeps the unit", func(t *testing.T) {
		home := t.TempDir()
		path := filepath.Join(home, ".config", "systemd", "user", linux.UnitName)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("[Service]\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		runner := &unitRunner{outputs: []platform.Output{{ExitCode: 1}}}
		item := linux.LoginItem{Runner: runner, Home: home, Systemctl: "/usr/bin/systemctl"}
		if err := item.Disable(context.Background()); err == nil {
			t.Fatal("disable exit 1 was reported as success")
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("unit was removed despite systemctl failure: %v", err)
		}
	})
}
