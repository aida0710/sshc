//go:build !windows && !android && !ios

package platform

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// シェル設定の入力待ちや大量出力で接続が止まらないための上限。
const loginShellPathTimeout = 5 * time.Second
const loginShellPathOutputLimit = 64 << 10
const loginShellPathWaitDelay = time.Second

// PATHは環境変数として/bin/shへ渡す。fishのPATH配列もここで文字列になり、
// 起動メッセージはNUL区切りの印の外に残る。実行するコマンドの本文は渡さない。
const loginShellPathCommand = `exec /bin/sh -c 'printf "\000sshc-path\000%s\000" "$PATH"'`
const loginShellPathMarker = "\x00sshc-path\x00"

// WithLoginShellPathは、ログインシェルのPATHだけを environment へ移した写しを返す。
//
// launchd や systemd が起動した engine のPATHは短く、利用者がシェルの設定で足した
// 場所（Homebrew、Docker Desktop など）を含まない。ProxyCommand と docker を
// 起動するときに使う。shell aliasや関数は移さない。
// 失敗時も起動元の環境を返し、既に使えているコマンドを妨げない。
func WithLoginShellPath(ctx context.Context, environment []string) ([]string, error) {
	if environment == nil {
		return nil, nil
	}
	lookup := func(name string) (string, bool) {
		for i := len(environment) - 1; i >= 0; i-- {
			if value, found := strings.CutPrefix(environment[i], name+"="); found {
				return value, true
			}
		}
		return "", false
	}
	shell, err := LoginShell(lookup)
	if err != nil {
		return environment, err
	}
	ctx, cancel := context.WithTimeout(ctx, loginShellPathTimeout)
	defer cancel()
	// -iは.zshrc、ログイン用argv[0]は.zprofileなどのPATH設定も読むため。
	process := exec.CommandContext(ctx, shell, "-i", "-c", loginShellPathCommand)
	process.Args[0] = LoginArgv0(shell)
	process.Env = environment
	if home, _ := lookup("HOME"); filepath.IsAbs(home) {
		process.Dir = home
	}
	process.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	process.Cancel = func() error {
		// 起動設定が待っている子プロセスもキャンセルする。
		err := syscall.Kill(-process.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	process.WaitDelay = loginShellPathWaitDelay
	output := &loginShellPathOutput{}
	process.Stdout = output
	// stdinは/dev/null、stderrは破棄する。起動設定の出力や秘密をSSHへ流さない。
	if err := process.Run(); err != nil {
		if ctx.Err() != nil {
			return environment, ctx.Err()
		}
		return environment, fmt.Errorf("read login shell PATH: %w", err)
	}
	path, err := output.path()
	if err != nil {
		return environment, err
	}
	updated := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		if !strings.HasPrefix(entry, "PATH=") {
			updated = append(updated, entry)
		}
	}
	return append(updated, "PATH="+path), nil
}

type loginShellPathOutput struct {
	buffer    bytes.Buffer
	truncated bool
}

func (output *loginShellPathOutput) Write(contents []byte) (int, error) {
	length := len(contents)
	remaining := loginShellPathOutputLimit - output.buffer.Len()
	if length > remaining {
		contents = contents[:remaining]
		output.truncated = true
	}
	_, _ = output.buffer.Write(contents)
	return length, nil
}

func (output *loginShellPathOutput) path() (string, error) {
	if output.truncated {
		return "", errors.New("login shell PATH output exceeded the limit")
	}
	_, framed, found := strings.Cut(output.buffer.String(), loginShellPathMarker)
	path, _, terminated := strings.Cut(framed, "\x00")
	if !found || !terminated || path == "" {
		return "", errors.New("login shell did not return a PATH")
	}
	return path, nil
}
