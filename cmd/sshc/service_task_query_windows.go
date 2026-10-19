//go:build windows

package main

import (
	"context"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Only reading uses COM: schtasks' Ready/Running/Disabled output is localized.
// Interface slots come from Microsoft's taskschd.h (ITaskService, ITaskFolder,
// IRegisteredTask); mutations continue to use the fixed schtasks runner.
const (
	// IRegisteredTask has 25 slots, the largest of the three queried interfaces.
	taskCOMMethodCount       = 25
	comRelease               = 2
	dispatchGetIDsOfNames    = 5
	dispatchInvoke           = 6
	taskServiceGetFolder     = 7
	taskFolderGetTask        = 13
	registeredTaskState      = 9
	registeredTaskXML        = 20
	classContextInprocServer = 1
	dispatchMethod           = 1
	hresultFileNotFound      = 0x80070002
)

type taskCOMObject struct{ methods *[taskCOMMethodCount]uintptr }
type taskDispatchParameters struct {
	arguments          uintptr
	namedArguments     uintptr
	argumentCount      uint32
	namedArgumentCount uint32
}

var (
	taskOle32            = windows.NewLazySystemDLL("ole32.dll")
	taskOleAut32         = windows.NewLazySystemDLL("oleaut32.dll")
	taskCoCreateInstance = taskOle32.NewProc("CoCreateInstance")
	taskCoUninitialize   = taskOle32.NewProc("CoUninitialize")
	taskSysAllocString   = taskOleAut32.NewProc("SysAllocString")
	taskSysFreeString    = taskOleAut32.NewProc("SysFreeString")
	taskSysStringLen     = taskOleAut32.NewProc("SysStringLen")
)

func taskCOMError(operation string, code uintptr) error {
	if int32(code) >= 0 {
		return nil
	}
	return fmt.Errorf("Task Scheduler %s failed (HRESULT 0x%08x)", operation, uint32(code))
}
func (object *taskCOMObject) release() {
	syscall.SyscallN(object.methods[comRelease], uintptr(unsafe.Pointer(object)))
}
func taskBSTR(value string) (uintptr, error) {
	text, err := windows.UTF16PtrFromString(value)
	if err != nil {
		return 0, err
	}
	allocated, _, _ := taskSysAllocString.Call(uintptr(unsafe.Pointer(text)))
	if allocated == 0 {
		return 0, windows.ERROR_NOT_ENOUGH_MEMORY
	}
	return allocated, nil
}
func connectTaskService(service *taskCOMObject) error {
	// IDispatch supplies the four optional empty VARIANT arguments to Connect.
	// This avoids architecture-dependent by-value VARIANT calling conventions.
	name, _ := windows.UTF16PtrFromString("Connect")
	var iid windows.GUID
	var method int32
	code, _, _ := syscall.SyscallN(service.methods[dispatchGetIDsOfNames], uintptr(unsafe.Pointer(service)), uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&name)), 1, 0, uintptr(unsafe.Pointer(&method)))
	if err := taskCOMError("resolve Connect", code); err != nil {
		return err
	}
	var parameters taskDispatchParameters
	code, _, _ = syscall.SyscallN(service.methods[dispatchInvoke], uintptr(unsafe.Pointer(service)), uintptr(method), uintptr(unsafe.Pointer(&iid)), 0, dispatchMethod, uintptr(unsafe.Pointer(&parameters)), 0, 0, 0)
	return taskCOMError("Connect", code)
}

func readWindowsServiceTask(ctx context.Context, taskName string) (serviceTaskSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return serviceTaskSnapshot{}, err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// S_FALSE means COM was already initialized on this thread and still needs a matching uninitialize.
	if err := windows.CoInitializeEx(0, windows.COINIT_MULTITHREADED); err != nil && err != syscall.Errno(1) {
		return serviceTaskSnapshot{}, err
	}
	defer taskCoUninitialize.Call()
	class, _ := windows.GUIDFromString("{0F87369F-A4E5-4CFC-BD3E-73E6154572DD}")
	iid, _ := windows.GUIDFromString("{2FABA4C7-4DA9-4013-9697-20CC3FD40F85}")
	var service *taskCOMObject
	code, _, _ := taskCoCreateInstance.Call(uintptr(unsafe.Pointer(&class)), 0, classContextInprocServer, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&service)))
	if err := taskCOMError("create service", code); err != nil {
		return serviceTaskSnapshot{}, err
	}
	defer service.release()
	if err := connectTaskService(service); err != nil {
		return serviceTaskSnapshot{}, err
	}
	root, err := taskBSTR(`\`)
	if err != nil {
		return serviceTaskSnapshot{}, err
	}
	defer taskSysFreeString.Call(root)
	var folder *taskCOMObject
	code, _, _ = syscall.SyscallN(service.methods[taskServiceGetFolder], uintptr(unsafe.Pointer(service)), root, uintptr(unsafe.Pointer(&folder)))
	if err := taskCOMError("open root folder", code); err != nil {
		return serviceTaskSnapshot{}, err
	}
	defer folder.release()
	name, err := taskBSTR(taskName)
	if err != nil {
		return serviceTaskSnapshot{}, err
	}
	defer taskSysFreeString.Call(name)
	var task *taskCOMObject
	code, _, _ = syscall.SyscallN(folder.methods[taskFolderGetTask], uintptr(unsafe.Pointer(folder)), name, uintptr(unsafe.Pointer(&task)))
	if uint32(code) == hresultFileNotFound {
		return serviceTaskSnapshot{}, nil
	}
	if err := taskCOMError("open sshc task", code); err != nil {
		return serviceTaskSnapshot{}, err
	}
	defer task.release()
	return readTaskCOMSnapshot(ctx, task)
}

func readTaskCOMSnapshot(ctx context.Context, task *taskCOMObject) (serviceTaskSnapshot, error) {
	var snapshot serviceTaskSnapshot
	code, _, _ := syscall.SyscallN(task.methods[registeredTaskState], uintptr(unsafe.Pointer(task)), uintptr(unsafe.Pointer(&snapshot.state)))
	if err := taskCOMError("read state", code); err != nil {
		return snapshot, err
	}
	var text *uint16
	code, _, _ = syscall.SyscallN(task.methods[registeredTaskXML], uintptr(unsafe.Pointer(task)), uintptr(unsafe.Pointer(&text)))
	if err := taskCOMError("read XML", code); err != nil {
		return snapshot, err
	}
	defer taskSysFreeString.Call(uintptr(unsafe.Pointer(text)))
	length, _, _ := taskSysStringLen.Call(uintptr(unsafe.Pointer(text)))
	definition, err := parseServiceTask(windows.UTF16ToString(unsafe.Slice(text, int(length))))
	if err != nil {
		return snapshot, err
	}
	if definition, err = definition.withAccountSIDs(accountSID); err != nil {
		return snapshot, err
	}
	snapshot.exists, snapshot.definition = true, definition
	return snapshot, ctx.Err()
}

// accountSID は、タスクスケジューラが返したアカウント名（例 WIN-XXXX\tester）を SID の文字列にする。
func accountSID(account string) (string, error) {
	sid, _, _, err := windows.LookupSID("", account)
	if err != nil {
		return "", fmt.Errorf("look up the scheduled task account %q: %w", account, err)
	}
	return sid.String(), nil
}
