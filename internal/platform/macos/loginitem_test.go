//go:build darwin

package macos_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshc/internal/platform"
	"sshc/internal/platform/macos"
)

// 本物のエージェントを読み込むテストはない。ランナーは launchctl が何を求められた
// はずかを記録するだけで、何もしない。
type recordingLaunchctl struct {
	commands []platform.Command
	outputs  []platform.Output
	errors   []error
}

func (r *recordingLaunchctl) RunOutput(_ context.Context, command platform.Command) (platform.Output, error) {
	r.commands = append(r.commands, command)
	index := len(r.commands) - 1
	var output platform.Output
	var err error
	if index < len(r.outputs) {
		output = r.outputs[index]
	}
	if index < len(r.errors) {
		err = r.errors[index]
	}
	return output, err
}

func TestEnablingWritesAnAgentThatOpensNoBrowserAndLogsNothing(t *testing.T) {
	home := t.TempDir()
	runner := &recordingLaunchctl{}
	item := macos.LoginItem{Runner: runner, Home: home, Launchctl: "/bin/launchctl"}

	if item.Enabled() {
		t.Fatal("a fresh home reports the agent as registered")
	}
	if err := item.Enable(context.Background(), "/Users/tester/.local/bin/sshc"); err != nil {
		t.Fatalf("Enable = %v", err)
	}
	if !item.Enabled() {
		t.Error("after Enable it is not registered")
	}

	path := filepath.Join(home, "Library", "LaunchAgents", macos.LoginItemLabel+".plist")
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("plist = %v, %v", info, err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	written := string(body)
	// ログイン時には何も開かず、何もリダイレクトしない。これが表示する URL は有効な
	// ブートストラップトークンを運び、ログファイルはそれを置く場所ではない。
	if !strings.Contains(written, "<string>-open=false</string>") {
		t.Errorf("the agent would open a browser at login: %s", written)
	}
	for _, absent := range []string{"StandardOutPath", "StandardErrorPath"} {
		if strings.Contains(written, absent) {
			t.Errorf("the agent redirects %s, which is where the bootstrap URL would land", absent)
		}
	}
	if !strings.Contains(written, "/Users/tester/.local/bin/sshc") {
		t.Errorf("the agent does not name the program: %s", written)
	}

	// boot in の前に boot out するので、パスが変わったときは古いものが残らずに
	// 置き換わる。
	if len(runner.commands) != 2 ||
		runner.commands[0].Arguments[0] != "bootout" || runner.commands[1].Arguments[0] != "bootstrap" {
		t.Errorf("launchctl calls = %#v", runner.commands)
	}

	if err := item.Disable(context.Background()); err != nil {
		t.Fatalf("Disable = %v", err)
	}
	if item.Enabled() {
		t.Error("after Disable it is still registered")
	}
	// 二度無効にすることは、呼び出し側が求めた状態である。
	if err := item.Disable(context.Background()); err != nil {
		t.Errorf("Disable twice = %v", err)
	}
}

// OutputRunnerは非zero終了をerrorではなくOutputへ入れる。ここを見落とすとlaunchdが
// 登録を拒否してもinstallは成功を報告する。
func TestEnableReportsLaunchctlExitFailures(t *testing.T) {
	t.Run("bootout failure", func(t *testing.T) {
		runner := &recordingLaunchctl{outputs: []platform.Output{{ExitCode: 5}}}
		item := macos.LoginItem{Runner: runner, Home: t.TempDir()}
		if err := item.Enable(context.Background(), "/Users/tester/.local/bin/sshc"); err == nil {
			t.Fatal("bootout exit 5 was reported as success")
		}
		if len(runner.commands) != 1 {
			t.Fatalf("commands after failed bootout = %#v", runner.commands)
		}
	})

	// 断られ続けたら諦める。**「10 回とも断られた」は「断られた」である。**
	t.Run("bootstrap failure", func(t *testing.T) {
		refused := []platform.Output{{ExitCode: 3}}
		for attempt := 0; attempt < 32; attempt++ {
			refused = append(refused, platform.Output{ExitCode: 5})
		}
		runner := &recordingLaunchctl{outputs: refused}
		item := macos.LoginItem{Runner: runner, Home: t.TempDir()}
		if err := item.Enable(context.Background(), "/Users/tester/.local/bin/sshc"); err == nil {
			t.Fatal("bootstrap exit 5 was reported as success")
		}
		if len(runner.commands) < 3 {
			t.Fatalf("it gave up without trying again: %#v", runner.commands)
		}
	})
}

// bootout は非同期である。
//
// **0 で戻ってきても、走っていたプロセスはまだ片付いていないことがある。**
// そこへ bootstrap すると launchd は断る——エンジンが動いている状態で
// `make install` を走らせると実際にそうなった。片付くのを待って、もう一度出す。
func TestEnableWaitsForTheOldServiceToFinishGoingAway(t *testing.T) {
	runner := &recordingLaunchctl{outputs: []platform.Output{
		{ExitCode: 0}, // bootout: 頼んだ。まだ終わってはいない
		{ExitCode: 5}, // bootstrap: まだ居る
		{ExitCode: 0}, // bootstrap: 片付いた
	}}
	item := macos.LoginItem{Runner: runner, Home: t.TempDir()}

	if err := item.Enable(context.Background(), "/Users/tester/.local/bin/sshc"); err != nil {
		t.Fatalf("Enable() = %v, want it to wait and try again", err)
	}
	if len(runner.commands) != 3 {
		t.Fatalf("commands = %#v", runner.commands)
	}
	if runner.commands[2].Arguments[0] != "bootstrap" {
		t.Fatalf("the second attempt was %#v", runner.commands[2])
	}
}

func TestDisableIgnoresOnlyAnAlreadyUnloadedService(t *testing.T) {
	t.Run("not loaded", func(t *testing.T) {
		home := t.TempDir()
		path := filepath.Join(home, "Library", "LaunchAgents", macos.LoginItemLabel+".plist")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("plist"), 0o600); err != nil {
			t.Fatal(err)
		}
		item := macos.LoginItem{
			Runner: &recordingLaunchctl{outputs: []platform.Output{{ExitCode: 3}}},
			Home:   home,
		}
		if err := item.Disable(context.Background()); err != nil {
			t.Fatalf("Disable = %v", err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("plist remains: %v", err)
		}
	})

	t.Run("real failure", func(t *testing.T) {
		home := t.TempDir()
		path := filepath.Join(home, "Library", "LaunchAgents", macos.LoginItemLabel+".plist")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("plist"), 0o600); err != nil {
			t.Fatal(err)
		}
		item := macos.LoginItem{
			Runner: &recordingLaunchctl{outputs: []platform.Output{{ExitCode: 5}}},
			Home:   home,
		}
		if err := item.Disable(context.Background()); err == nil {
			t.Fatal("bootout exit 5 was reported as success")
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("plist was removed despite bootout failure: %v", err)
		}
	})
}

func TestEnablingRefusesAProgramLaunchdWouldHaveToFind(t *testing.T) {
	item := macos.LoginItem{Runner: &recordingLaunchctl{}, Home: t.TempDir()}
	if err := item.Enable(context.Background(), "sshc"); err == nil {
		t.Error("a relative program was accepted")
	}
}

func TestRegisteredDistinguishesAbsentPresentAndUnreadableAgentState(t *testing.T) {
	t.Run("absent", func(t *testing.T) {
		item := macos.LoginItem{Home: t.TempDir()}
		registered, err := item.Registered()
		if err != nil || registered {
			t.Fatalf("Registered = %v, %v; want false, nil", registered, err)
		}
	})

	t.Run("present", func(t *testing.T) {
		home := t.TempDir()
		path := filepath.Join(home, "Library", "LaunchAgents", macos.LoginItemLabel+".plist")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("plist"), 0o600); err != nil {
			t.Fatal(err)
		}
		item := macos.LoginItem{Home: home}
		registered, err := item.Registered()
		if err != nil || !registered {
			t.Fatalf("Registered = %v, %v; want true, nil", registered, err)
		}
	})

	t.Run("unknown", func(t *testing.T) {
		home := t.TempDir()
		if err := os.WriteFile(filepath.Join(home, "Library"), []byte("not a directory"), 0o600); err != nil {
			t.Fatal(err)
		}
		item := macos.LoginItem{Home: home}
		registered, err := item.Registered()
		if err == nil || registered {
			t.Fatalf("Registered = %v, %v; want false and an error", registered, err)
		}
	})
}
