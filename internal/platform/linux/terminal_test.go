//go:build linux

package linux_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"sshc/internal/platform"
	"sshc/internal/platform/linux"
)

// terminalRunner は、実行されたはずのコマンドを記録する。このパッケージのどの
// テストも本物の端末を開かない。テストから開けば、デスクで動いている何かに
// alias や helperPath を渡すことになる。
type terminalRunner struct{ commands []platform.Command }

func (runner *terminalRunner) RunOutput(_ context.Context, command platform.Command) (platform.Output, error) {
	runner.commands = append(runner.commands, command)
	return platform.Output{}, nil
}

// 端末の表は持たない。
//
// macOS では「CLI を持たない端末は Terminal.app と iTerm2 の二つで打ち止め」と
// 言い切れるので profile の表が意味を持つ。Linux では端末が乱立していて、実行する
// コマンドの渡し方も端末ごとに違う。表を用意すれば、そこに無い端末を使う人には
// 効かず、そこにある端末でも規約を取り違えれば黙って壊れる。推測しない。
func TestOnlyTheCustomTerminalIsOffered(t *testing.T) {
	terminal := linux.NewTerminal(&terminalRunner{})
	if got := terminal.Applications(); len(got) != 0 {
		t.Fatalf("Applications() = %#v, want none", got)
	}
	want := []platform.TerminalAvailability{{ID: platform.TerminalCustom, Installed: true}}
	if got := terminal.Terminals(); !slices.Equal(got, want) {
		t.Fatalf("Terminals() = %#v, want %#v", got, want)
	}
}

// choice が無ければ、開く先を推測しない。既定はどちらの経路にも無い。
func TestLaunchAndLaunchWithPasswordHaveNoDefault(t *testing.T) {
	runner := &terminalRunner{}
	terminal := linux.NewTerminal(runner)
	if err := terminal.Launch(context.Background(), "bastion"); err != platform.ErrTerminalApplication {
		t.Errorf("Launch() = %v, want ErrTerminalApplication", err)
	}
	if err := terminal.LaunchWithPassword(
		context.Background(), "bastion", "/usr/bin/sshc", "http://127.0.0.1:0", "token",
	); err != platform.ErrTerminalApplication {
		t.Errorf("LaunchWithPassword() = %v, want ErrTerminalApplication", err)
	}
	if len(runner.commands) != 0 {
		t.Error("the process seam was reached anyway")
	}
}

// 利用者が書いたコマンドが、そのまま argv として届く。シェルは介在しない。
func TestLaunchRunsTheChosenProgramWithTheAliasLast(t *testing.T) {
	runner := &terminalRunner{}
	terminal := linux.NewTerminal(runner)
	choice := platform.TerminalChoice{
		ID:          platform.TerminalCustom,
		Application: "/usr/bin/foot",
		Arguments:   []string{"-e"},
	}

	if err := terminal.LaunchIn(context.Background(), choice, "bastion"); err != nil {
		t.Fatal(err)
	}
	if len(runner.commands) != 1 {
		t.Fatalf("commands = %d, want 1", len(runner.commands))
	}
	command := runner.commands[0]
	if command.Path != "/usr/bin/foot" {
		t.Errorf("Path = %q", command.Path)
	}
	want := []string{"-e", "/usr/bin/ssh", "--", "bastion"}
	if !slices.Equal(command.Arguments, want) {
		t.Errorf("Arguments = %#v, want %#v", command.Arguments, want)
	}
	// choice.Arguments 自体は書き換えられていない。
	if !slices.Equal(choice.Arguments, []string{"-e"}) {
		t.Errorf("choice.Arguments mutated to %#v", choice.Arguments)
	}
}

// パスワード経路も同じ argv 規約に従うが、末尾は ssh ではなく helperPath である。
func TestLaunchWithPasswordRunsTheHelperWithTheAliasLast(t *testing.T) {
	runner := &terminalRunner{}
	terminal := linux.NewTerminal(runner)
	choice := platform.TerminalChoice{
		ID:          platform.TerminalCustom,
		Application: "/usr/bin/foot",
		Arguments:   []string{"-e"},
	}

	err := terminal.LaunchWithPasswordIn(
		context.Background(), choice, "bastion", "/usr/local/bin/sshc", "http://127.0.0.1:4100", "s3cr3t",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.commands) != 1 {
		t.Fatalf("commands = %d, want 1", len(runner.commands))
	}
	command := runner.commands[0]
	if command.Path != "/usr/bin/foot" {
		t.Errorf("Path = %q", command.Path)
	}
	want := []string{"-e", "/usr/local/bin/sshc", "bastion"}
	if !slices.Equal(command.Arguments, want) {
		t.Errorf("Arguments = %#v, want %#v", command.Arguments, want)
	}
}

// 安全な文字集合の外にある alias は起動しない。
func TestLaunchRefusesAnUnsafeAlias(t *testing.T) {
	runner := &terminalRunner{}
	choice := platform.TerminalChoice{
		ID: platform.TerminalCustom, Application: "/usr/bin/foot", Arguments: []string{"-e"},
	}
	if err := linux.NewTerminal(runner).LaunchIn(context.Background(), choice, "a;rm -rf /"); err == nil {
		t.Fatal("an unsafe alias was launched")
	}
	if len(runner.commands) != 0 {
		t.Error("the process seam was reached anyway")
	}
}

// custom 以外の ID は、この端末では開けない。定義済みの端末を差し出していない。
func TestLaunchInRefusesANonCustomChoice(t *testing.T) {
	runner := &terminalRunner{}
	choice := platform.TerminalChoice{ID: platform.TerminalApple}
	err := linux.NewTerminal(runner).LaunchIn(context.Background(), choice, "bastion")
	if err != platform.ErrTerminalApplication {
		t.Errorf("LaunchIn() = %v, want ErrTerminalApplication", err)
	}
	if len(runner.commands) != 0 {
		t.Error("the process seam was reached anyway")
	}
}

// helperPath が絶対でなければ、systemd 側と同じ理由で拒否する。他人が供給しうる
// プログラムを PATH 経由で探させてはならない。
func TestLaunchWithPasswordInRefusesARelativeHelperPath(t *testing.T) {
	runner := &terminalRunner{}
	choice := platform.TerminalChoice{
		ID: platform.TerminalCustom, Application: "/usr/bin/foot", Arguments: []string{"-e"},
	}
	err := linux.NewTerminal(runner).LaunchWithPasswordIn(
		context.Background(), choice, "bastion", "sshc", "http://127.0.0.1:4100", "s3cr3t",
	)
	if err != linux.ErrHelperPathNotAbsolute {
		t.Errorf("LaunchWithPasswordIn() = %v, want ErrHelperPathNotAbsolute", err)
	}
	if len(runner.commands) != 0 {
		t.Error("the process seam was reached anyway")
	}
}

// トークンと endpoint は、コマンド行にも環境にも決して現れない。ウィンドウが
// 実行するのは helperPath と alias だけで、トークンはそのプロセスが handoff
// から自分で取る。ps に見えれば、この利用者として動くすべてのプロセスに漏れる。
func TestLaunchWithPasswordNeverCarriesTheTokenOrEndpoint(t *testing.T) {
	runner := &terminalRunner{}
	choice := platform.TerminalChoice{
		ID: platform.TerminalCustom, Application: "/usr/bin/foot", Arguments: []string{"-e"},
	}
	const endpoint = "http://127.0.0.1:4100/handoff"
	const token = "s3cr3t-one-time-token"

	err := linux.NewTerminal(runner).LaunchWithPasswordIn(
		context.Background(), choice, "bastion", "/usr/local/bin/sshc", endpoint, token,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.commands) != 1 {
		t.Fatalf("commands = %d, want 1", len(runner.commands))
	}
	command := runner.commands[0]
	if strings.Contains(command.Path, token) || strings.Contains(command.Path, endpoint) {
		t.Errorf("Path = %q, carries the secret", command.Path)
	}
	for _, argument := range command.Arguments {
		if strings.Contains(argument, token) || strings.Contains(argument, endpoint) {
			t.Errorf("Arguments = %#v, carries the secret", command.Arguments)
		}
	}
	for _, variable := range command.Env {
		if strings.Contains(variable, token) || strings.Contains(variable, endpoint) {
			t.Errorf("Env = %#v, carries the secret", command.Env)
		}
	}
}
