//go:build !windows

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// cancelWhenReady は、installerの代わりのshellがreadyへ書き込んだら（準備ができたら）ctxを取り消す。
// FIFOの読み出しは書き手が開いて閉じるまで待つので、時間で推測せずに済む。
func cancelWhenReady(t *testing.T, ready string, cancel context.CancelFunc) {
	t.Helper()
	if err := syscall.Mkfifo(ready, 0o600); err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = os.ReadFile(ready)
		cancel()
	}()
}

// 取り消されたinstallerには、まずSIGTERMを送ってtrapで後始末させる。SIGKILLはtrapで
// 受けられないので、install.shが置き先に作った一時ファイルが残る。
func TestCanceledUpdateCommandLetsTheInstallerCleanUp(t *testing.T) {
	directory := t.TempDir()
	ready := filepath.Join(directory, "ready")
	cleaned := filepath.Join(directory, "cleaned")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancelWhenReady(t, ready, cancel)

	// 準備ができたと伝えるのは、downloadの代わりのforegroundのprocess自身である。shellが
	// 次のcommandへ進む前にSIGTERMが届いてtrapを取りこぼす競合を、テストに持ち込まない。
	script := `trap 'echo cleaned > "$SSHC_TEST_CLEANED"; exit 130' TERM
sh -c 'echo ready > "$SSHC_TEST_READY"; exec sleep 30'
echo finished
`
	environment := append(os.Environ(), "SSHC_TEST_READY="+ready, "SSHC_TEST_CLEANED="+cleaned)
	err := systemInstallationCommands{}.Run(ctx, installationProcess{name: "sh", args: []string{"-c", script}, environment: environment})
	if err == nil {
		t.Fatal("the canceled installer reported success")
	}
	if _, err := os.Stat(cleaned); err != nil {
		t.Fatalf("the installer's TERM trap did not run: %v", err)
	}
}

// SIGTERMを無視して残ったものは、installerが終わった後にSIGKILLで止める。shellだけが
// 終わって、downloadがprocess groupに残ることはない。
func TestCanceledUpdateCommandLeavesNothingInItsProcessGroup(t *testing.T) {
	directory := t.TempDir()
	ready := filepath.Join(directory, "ready")
	leftover := filepath.Join(directory, "leftover.pid")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancelWhenReady(t, ready, cancel)

	script := `sh -c 'trap "" TERM; echo $$ > "$SSHC_TEST_LEFTOVER"; exec sleep 30' &
while [ ! -s "$SSHC_TEST_LEFTOVER" ]; do sleep 0.05; done
echo ready > "$SSHC_TEST_READY"
wait
`
	environment := append(os.Environ(), "SSHC_TEST_READY="+ready, "SSHC_TEST_LEFTOVER="+leftover)
	if err := (systemInstallationCommands{}).Run(ctx, installationProcess{name: "sh", args: []string{"-c", script}, environment: environment}); err == nil {
		t.Fatal("the canceled installer reported success")
	}
	body, err := os.ReadFile(leftover)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	// SIGKILLの後、孤児になったprocessをinitが回収するまでは少しかかる。
	deadline := time.Now().Add(10 * time.Second)
	for {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		if time.Now().After(deadline) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("process %d that ignored SIGTERM is still running after the canceled update", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
