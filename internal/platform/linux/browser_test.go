//go:build linux

package linux_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"sshc/internal/platform"
	"sshc/internal/platform/linux"
)

// browserRunner は、実行されたはずのコマンドを記録する。このパッケージのどのテストも
// 本物のブラウザを開かない。テストから開けば、デスクで動いている何かに生きた
// bootstrap トークンを渡すことになる。
type browserRunner struct{ commands []platform.Command }

func (runner *browserRunner) RunOutput(_ context.Context, command platform.Command) (platform.Output, error) {
	runner.commands = append(runner.commands, command)
	return platform.Output{}, nil
}

func TestBrowserRunsXdgOpenByAbsolutePath(t *testing.T) {
	runner := &browserRunner{}
	target := "http://127.0.0.1:43123/#bootstrap=abc;$(touch%20/tmp/nope)"

	if err := linux.NewBrowser(runner).Open(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	if len(runner.commands) != 1 {
		t.Fatalf("commands = %d, want 1", len(runner.commands))
	}
	command := runner.commands[0]
	if command.Path != "/usr/bin/xdg-open" {
		t.Errorf("Path = %q, want the absolute path so PATH is never consulted", command.Path)
	}
	if !slices.Equal([]string{target}, command.Arguments) {
		t.Errorf("Arguments = %#v, want the URL as one element", command.Arguments)
	}
}

// ループバックの http 以外は開かない。URL は生きた bootstrap トークンを運ぶ。
func TestBrowserRefusesAnythingButLoopbackHTTP(t *testing.T) {
	for _, target := range []string{
		"https://example.com/", "http://example.com/",
		"http://192.168.1.10/", "file:///etc/passwd",
	} {
		runner := &browserRunner{}
		if err := linux.NewBrowser(runner).Open(context.Background(), target); !errors.Is(err, linux.ErrUnsafeBrowserURL) {
			t.Errorf("Open(%q) = %v, want ErrUnsafeBrowserURL", target, err)
		}
		if len(runner.commands) != 0 {
			t.Errorf("Open(%q) reached the process seam anyway", target)
		}
	}
}
