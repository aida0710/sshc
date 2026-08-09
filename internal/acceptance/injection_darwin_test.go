package acceptance_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"sshc/internal/platform/macos"
)

// このテストが injection_test.go から分かれて別ファイルにあるのは、macOS が
// 組み立てる AppleScript の文字そのものを検証するからである。他の
// プラットフォームにはこれに対応するものが無い。ファイル名の "_darwin"
// 接尾辞そのものが Go のビルド制約になるので、ここには //go:build を
// 重ねて書かない。injection_test.go に残る他のテストは argv や path の
// 侵入防御であり、どの OS でも成り立たなければならない。
func TestTerminalLaunchNeverBuildsAppleScriptFromInput(t *testing.T) {
	// スクリプト自体には置換点が一切あってはならない。
	//
	// plan のリストは AppleScript の連結演算子も禁じていたが、
	// コミットされたスクリプトはそれを使って *定数* の prefix を
	// `quoted form of targetAlias` に連結しており、これは危険な方ではなく
	// 安全な構成である。存在してはならないのは、呼び出し元のテキストが
	// スクリプトへと書式化される点であり、これら 4 通りの綴りはまさにそれに当たる。
	for _, forbidden := range []string{"%s", "%v", "%q", "${"} {
		if strings.Contains(macos.TerminalScript, forbidden) {
			t.Fatalf("TerminalScript contains a substitution point %q", forbidden)
		}
	}
	if !strings.Contains(macos.TerminalScript, "quoted form of") {
		t.Fatal("TerminalScript does not quote its argument for the shell that runs it")
	}
	if !strings.Contains(macos.TerminalScript, "item 1 of argv") {
		t.Fatal("TerminalScript does not take the alias from argv")
	}

	runner := &recordingRunner{}
	terminal := macos.Terminal{Runner: runner, Program: "/usr/bin/osascript", Timeout: 5 * time.Second}

	// Positive control。
	if err := terminal.Launch(context.Background(), "bastion"); err != nil {
		t.Fatalf("Launch(bastion) = %v", err)
	}
	recorded := runner.recorded()
	if len(recorded) != 1 {
		t.Fatalf("a safe alias produced %d commands, want 1", len(recorded))
	}
	command := recorded[0]
	if command.Path != "/usr/bin/osascript" {
		t.Fatalf("path = %q", command.Path)
	}
	if len(command.Arguments) != 2 || command.Arguments[0] != "-" || command.Arguments[1] != "bastion" {
		t.Fatalf("arguments = %#v, want [- bastion]", command.Arguments)
	}
	if string(command.Stdin) != macos.TerminalScript {
		t.Fatal("the script sent on stdin is not the package constant")
	}
	if strings.Contains(string(command.Stdin), "bastion") {
		t.Fatal("the alias was concatenated into the script")
	}

	// Hostile half。
	for _, hostile := range hostileArguments {
		t.Run(quoteForName(hostile), func(t *testing.T) {
			runner.reset()
			err := terminal.Launch(context.Background(), hostile)
			if err == nil {
				t.Fatalf("Launch(%q) was accepted", hostile)
			}
			if commands := runner.recorded(); len(commands) != 0 {
				t.Fatalf("a refused launch still ran %#v", commands)
			}
		})
	}
}
