package sshclient

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// PATH では探さない。探せば、`cmd.exe` という名前の別のものを起動しうる。
// Windows 自身が置いた場所だけを見る。
func TestTheCommandInterpreterComesFromWindowsItself(t *testing.T) {
	root := t.TempDir()
	system32 := filepath.Join(root, "System32")
	if err := os.MkdirAll(system32, 0o700); err != nil {
		t.Fatal(err)
	}
	shell := filepath.Join(system32, "cmd.exe")
	if err := os.WriteFile(shell, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}

	// ComSpec が指しているなら、それが結果である。
	found, err := commandInterpreter(func(name string) (string, bool) {
		if name == "ComSpec" {
			return shell, true
		}
		return "", false
	})
	if err != nil || found != shell {
		t.Errorf("commandInterpreter = %q, %v; want %q", found, err, shell)
	}

	// ComSpec が無ければ SystemRoot から組み立てる。
	found, err = commandInterpreter(func(name string) (string, bool) {
		if name == "SystemRoot" {
			return root, true
		}
		return "", false
	})
	if err != nil || found != shell {
		t.Errorf("without ComSpec = %q, %v; want %q", found, err, shell)
	}

	// 相対パスの ComSpec は受け取らない。そこを差し替えられれば、
	// 起動するのは Windows が置いたものではなくなる。
	if _, err := commandInterpreter(func(name string) (string, bool) {
		if name == "ComSpec" {
			return "cmd.exe", true
		}
		return "", false
	}); !errors.Is(err, ErrNoInterpreter) {
		t.Errorf("a relative ComSpec = %v, want ErrNoInterpreter", err)
	}

	// 何も無ければ、起動する相手が居ないと言う。
	if _, err := commandInterpreter(func(string) (string, bool) { return "", false }); !errors.Is(err, ErrNoInterpreter) {
		t.Errorf("with nothing to go on = %v, want ErrNoInterpreter", err)
	}
}
