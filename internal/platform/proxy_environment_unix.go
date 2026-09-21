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
const proxyPathTimeout = 5 * time.Second
const proxyPathOutputLimit = 64 << 10
const proxyPathWaitDelay = time.Second

// PATHは環境変数として/bin/shへ渡す。fishのPATH配列もここで文字列になり、
// 起動メッセージはNUL区切りの印の外に残る。ProxyCommand本文は渡さない。
const proxyPathCommand = `exec /bin/sh -c 'printf "\000sshc-path\000%s\000" "$PATH"'`
const proxyPathMarker = "\x00sshc-path\x00"

// ProxyEnvironmentはログインシェルのPATHだけをProxyCommandの環境へ移す。
// shell aliasや関数は移さず、コマンドの解釈は引き続き/bin/shが担当する。
// 失敗時も起動元の環境を返し、既に使えているProxyCommandを妨げない。
func ProxyEnvironment(ctx context.Context, environment []string) ([]string, error) {
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
	ctx, cancel := context.WithTimeout(ctx, proxyPathTimeout)
	defer cancel()
	// -iは.zshrc、ログイン用argv[0]は.zprofileなどのPATH設定も読むため。
	process := exec.CommandContext(ctx, shell, "-i", "-c", proxyPathCommand)
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
	process.WaitDelay = proxyPathWaitDelay
	output := &proxyPathOutput{}
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

type proxyPathOutput struct {
	buffer    bytes.Buffer
	truncated bool
}

func (output *proxyPathOutput) Write(contents []byte) (int, error) {
	length := len(contents)
	remaining := proxyPathOutputLimit - output.buffer.Len()
	if length > remaining {
		contents = contents[:remaining]
		output.truncated = true
	}
	_, _ = output.buffer.Write(contents)
	return length, nil
}

func (output *proxyPathOutput) path() (string, error) {
	if output.truncated {
		return "", errors.New("login shell PATH output exceeded the limit")
	}
	_, framed, found := strings.Cut(output.buffer.String(), proxyPathMarker)
	path, _, terminated := strings.Cut(framed, "\x00")
	if !found || !terminated || path == "" {
		return "", errors.New("login shell did not return a PATH")
	}
	return path, nil
}
