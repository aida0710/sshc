//go:build windows

// Package windowsjob は、起動したプロセスとその子孫をまとめて止める Job Object を作る。
//
// Windows の TerminateProcess は子を止めない。cmd.exe のように子を起こして待つ
// プログラムを止めるには、木ごと Job Object に入れておき、Job Object を止める。
package windowsjob

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// KillOnClose は、process を kill-on-close の Job Object に入れ、その Job Object を返す。
//
// 返した handle を閉じると、中に残っているプロセスはすべて止まる。process が
// 起こす子も同じ Job Object に入るので、子を起こす前に呼ぶこと（CREATE_SUSPENDED
// で起動し、ここで入れてから動かす）。
func KillOnClose(process windows.Handle) (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, fmt.Errorf("create the job object: %w", err)
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	// SetInformationJobObject は (ret, err) を返し、ret == 0 が失敗である。
	if ret, err := windows.SetInformationJobObject(
		job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)),
	); ret == 0 {
		_ = windows.CloseHandle(job)
		return 0, fmt.Errorf("limit the job object: %w", err)
	}
	// 入れられないことは致命である。回避しない。入れ子の Job Object は Windows 8
	// 以降で使える。
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		_ = windows.CloseHandle(job)
		return 0, fmt.Errorf("put the process in its job object: %w", err)
	}
	return job, nil
}
