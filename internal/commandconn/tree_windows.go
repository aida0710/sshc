//go:build windows

package commandconn

import (
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"sshc/internal/platform/windowsjob"
)

// forcedExitCode は、TerminateJobObject が木のすべてのプロセスに刻む値である。
const forcedExitCode = 1

// prepareTree は、プログラムを止めた状態で起動させる。Job Object に入れる前に
// cmd.exe が子を起こすと、その子は Job Object から漏れるからである。
func prepareTree(process *exec.Cmd) {
	if process.SysProcAttr == nil {
		process.SysProcAttr = &syscall.SysProcAttr{}
	}
	process.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED
}

// adoptTree は、止めた状態で起動したプログラムを kill-on-close の Job Object に
// 入れてから動かす。失敗したら、プログラムは止めずに返す。止めるのは呼び出し側である。
func adoptTree(process *exec.Cmd) (processTree, error) {
	pid := uint32(process.Process.Pid)
	handle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return nil, fmt.Errorf("open the started process: %w", err)
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	job, err := windowsjob.KillOnClose(handle)
	if err != nil {
		return nil, err
	}
	if err := resumeProcess(pid); err != nil {
		_ = windows.CloseHandle(job)
		return nil, err
	}
	return &jobTree{job: job}, nil
}

// errNoThread は、止めて起動したプロセスに再開するスレッドが見つからないことを表す。
var errNoThread = errors.New("the started process has no thread to resume")

// resumeProcess は、CREATE_SUSPENDED で起動したプロセスのスレッドを動かす。
//
// os/exec は最初のスレッドの handle を渡さないので、スナップショットから探す。
// 止めて起動したプロセスのスレッドは、最初のひとつしかない。
func resumeProcess(pid uint32) error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return fmt.Errorf("list the threads: %w", err)
	}
	defer func() { _ = windows.CloseHandle(snapshot) }()
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	resumed := false
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != pid {
			continue
		}
		thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if err != nil {
			return fmt.Errorf("open the thread of the started process: %w", err)
		}
		_, err = windows.ResumeThread(thread)
		_ = windows.CloseHandle(thread)
		if err != nil {
			return fmt.Errorf("resume the started process: %w", err)
		}
		resumed = true
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return fmt.Errorf("list the threads: %w", err)
	}
	if !resumed {
		return errNoThread
	}
	return nil
}

// jobTree は、Job Object でまとめた木である。
type jobTree struct {
	job         windows.Handle
	releaseOnce sync.Once
}

func (tree *jobTree) kill() error {
	return windows.TerminateJobObject(tree.job, forcedExitCode)
}

// release は Job Object を閉じる。kill-on-close なので、残っている子孫もここで止まる。
func (tree *jobTree) release() {
	tree.releaseOnce.Do(func() { _ = windows.CloseHandle(tree.job) })
}
