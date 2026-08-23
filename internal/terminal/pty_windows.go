//go:build windows

package terminal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// forcedExitCode は、TerminateJobObject が木のすべてのプロセスに刻む値である。
const forcedExitCode = 1

// conpty は、CreatePseudoConsole の在否を呼ぶ前に確かめるための探りである。
var conpty = windows.NewLazySystemDLL("kernel32.dll").NewProc("CreatePseudoConsole")

// ErrPseudoConsoleUnavailable は Windows が擬似コンソールに対応していないことを示す。
var ErrPseudoConsoleUnavailable = errors.New(
	"this version of Windows has no pseudoconsole; sshc needs Windows 10 version 1809 or later")

// WindowsStarter は Windows の擬似コンソールを確保する。
type WindowsStarter struct{}

func NewStarter() Starter { return WindowsStarter{} }

func (WindowsStarter) Start(ctx context.Context, command Command, size Size) (Process, error) {
	if err := conpty.Find(); err != nil {
		return nil, ErrPseudoConsoleUnavailable
	}
	if !size.Valid() {
		return nil, ErrInvalidSize
	}
	if command.Path == "" || !filepath.IsAbs(command.Path) {
		return nil, errors.New("terminal: the program path must be absolute")
	}
	// 取り消された確保は、何も残さない。
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	environment, err := environmentBlock(command.Env)
	if err != nil {
		return nil, err
	}

	process, err := startPseudoConsole(command, size, environment)
	if err != nil {
		return nil, err
	}
	return process, nil
}

// startPseudoConsole は、擬似コンソール・子プロセス・ジョブを組み立てる。
func startPseudoConsole(command Command, size Size, environment *uint16) (_ Process, err error) {
	// 二組のパイプ。子側の端は擬似コンソールへ渡し、こちら側の端を保持する。
	var inputRead, inputWrite windows.Handle
	if err := windows.CreatePipe(&inputRead, &inputWrite, nil, 0); err != nil {
		return nil, fmt.Errorf("terminal: create the console input pipe: %w", err)
	}
	var outputRead, outputWrite windows.Handle
	if err := windows.CreatePipe(&outputRead, &outputWrite, nil, 0); err != nil {
		windows.CloseHandle(inputRead)
		windows.CloseHandle(inputWrite)
		return nil, fmt.Errorf("terminal: create the console output pipe: %w", err)
	}

	var console windows.Handle
	consoleErr := windows.CreatePseudoConsole(
		windows.Coord{X: int16(size.Cols), Y: int16(size.Rows)},
		inputRead, outputWrite, 0, &console,
	)
	// 擬似コンソールへ渡したパイプ端をここで解放する。
	windows.CloseHandle(inputRead)
	windows.CloseHandle(outputWrite)
	if consoleErr != nil {
		windows.CloseHandle(inputWrite)
		windows.CloseHandle(outputRead)
		return nil, fmt.Errorf("terminal: create the pseudoconsole: %w", consoleErr)
	}
	defer func() {
		if err == nil {
			return
		}
		// 呼び出し側のパイプ端を先に解放し、コンソールは別 goroutine で閉じる。
		windows.CloseHandle(inputWrite)
		windows.CloseHandle(outputRead)
		go windows.ClosePseudoConsole(console)
	}()

	attributes, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return nil, fmt.Errorf("terminal: allocate the process attributes: %w", err)
	}
	defer attributes.Delete()
	if err := attributes.Update(
		windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
		*(*unsafe.Pointer)(unsafe.Pointer(&console)),
		unsafe.Sizeof(console),
	); err != nil {
		return nil, fmt.Errorf("terminal: attach the pseudoconsole: %w", err)
	}

	startup := windows.StartupInfoEx{ProcThreadAttributeList: attributes.List()}
	startup.Cb = uint32(unsafe.Sizeof(startup))
	// STARTF_USESTDHANDLES を、ハンドルを NULL のまま立てる。
	startup.Flags |= windows.STARTF_USESTDHANDLES

	applicationName, err := windows.UTF16PtrFromString(command.Path)
	if err != nil {
		return nil, fmt.Errorf("terminal: the program path is not usable: %w", err)
	}
	commandLine, err := windows.UTF16PtrFromString(composeCommandLine(command))
	if err != nil {
		return nil, fmt.Errorf("terminal: the command line is not usable: %w", err)
	}
	var directory *uint16
	if command.Dir != "" {
		if directory, err = windows.UTF16PtrFromString(command.Dir); err != nil {
			return nil, fmt.Errorf("terminal: the working directory is not usable: %w", err)
		}
	}

	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_SUSPENDED)
	if environment != nil {
		// これが無いと、カーネルは UTF-16 の環境ブロックを ANSI として読む。
		flags |= windows.CREATE_UNICODE_ENVIRONMENT
	}
	var information windows.ProcessInformation
	if err := windows.CreateProcess(
		applicationName, commandLine, nil, nil,
		// 何も継がせない。 子がコンソールを受け取る道は属性リストだけである。
		false,
		flags, environment, directory, &startup.StartupInfo, &information,
	); err != nil {
		return nil, fmt.Errorf("terminal: start %s: %w", command.Path, err)
	}

	job, err := assignToJob(information.Process)
	if err != nil {
		windows.TerminateProcess(information.Process, forcedExitCode)
		windows.CloseHandle(information.Thread)
		windows.CloseHandle(information.Process)
		return nil, err
	}
	if _, err := windows.ResumeThread(information.Thread); err != nil {
		windows.TerminateProcess(information.Process, forcedExitCode)
		windows.CloseHandle(job)
		windows.CloseHandle(information.Thread)
		windows.CloseHandle(information.Process)
		return nil, fmt.Errorf("terminal: resume %s: %w", command.Path, err)
	}
	windows.CloseHandle(information.Thread)

	started := &windowsProcess{
		input:   os.NewFile(uintptr(inputWrite), "conpty-input"),
		output:  os.NewFile(uintptr(outputRead), "conpty-output"),
		console: console,
		job:     job,
		process: information.Process,
		exited:  make(chan struct{}),
	}
	go started.watch()
	return started, nil
}

// assignToJob はプロセスツリーを一括終了できる Job Object を作成する。
func assignToJob(process windows.Handle) (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, fmt.Errorf("terminal: create the job object: %w", err)
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	// SetInformationJobObject は (ret, err) を返し、ret == 0 が失敗である。
	if ret, err := windows.SetInformationJobObject(
		job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)),
	); ret == 0 {
		windows.CloseHandle(job)
		return 0, fmt.Errorf("terminal: limit the job object: %w", err)
	}
	// 入れられないことは致命である。回避しない。 入れ子のジョブは Windows 8
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		windows.CloseHandle(job)
		return 0, fmt.Errorf("terminal: put the console process in its job: %w", err)
	}
	return job, nil
}

// composeCommandLine は argv を Windows コマンドライン形式へ変換する。
func composeCommandLine(command Command) string {
	argv0 := command.Argv0
	if argv0 == "" {
		argv0 = command.Path
	}
	return windows.ComposeCommandLine(append([]string{argv0}, command.Arguments...))
}

// environmentBlock は、UTF-16 の環境ブロックを組み立てる。
func environmentBlock(environment []string) (*uint16, error) {
	if environment == nil {
		return nil, nil
	}
	var builder []uint16
	for _, entry := range environment {
		if strings.IndexByte(entry, 0) >= 0 || !strings.Contains(entry, "=") {
			return nil, fmt.Errorf("terminal: refusing an unusable environment entry")
		}
		encoded, err := windows.UTF16FromString(entry)
		if err != nil {
			return nil, fmt.Errorf("terminal: the environment is not usable: %w", err)
		}
		builder = append(builder, encoded...)
	}
	if len(builder) == 0 {
		builder = append(builder, 0)
	}
	builder = append(builder, 0)
	return &builder[0], nil
}

// windowsProcess は、擬似コンソールひとつと、その中の木である。
type windowsProcess struct {
	input  *os.File
	output *os.File

	mutex           sync.RWMutex
	console         windows.Handle
	consoleReleased bool
	job             windows.Handle
	jobReleased     bool

	process windows.Handle
	forced  bool

	// exited は、監視処理が終了理由を書き終えたことを示す。
	exited chan struct{}
	exit   ExitInfo
}

func (p *windowsProcess) wasForced() bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.forced
}

func (p *windowsProcess) Read(b []byte) (int, error)  { return p.output.Read(b) }
func (p *windowsProcess) Write(b []byte) (int, error) { return p.input.Write(b) }

// Resize は解放前のコンソールだけを変更する。
func (p *windowsProcess) Resize(size Size) error {
	if !size.Valid() {
		return ErrInvalidSize
	}
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	if p.consoleReleased {
		// 解放済みのコンソールでは何もしない。
		return nil
	}
	return windows.ResizePseudoConsole(p.console, windows.Coord{X: int16(size.Cols), Y: int16(size.Rows)})
}

// Hangup は、木に終わってほしいという意思である。
func (p *windowsProcess) Hangup() error {
	p.releaseConsole()
	return nil
}

// ForceClose は、締切に達した合図である。待たない。
func (p *windowsProcess) ForceClose() error {
	p.mutex.Lock()
	p.forced = true
	var terminateErr error
	if !p.jobReleased {
		// Job Object を終了し、コンソールに接続していない子孫も停止する。
		terminateErr = windows.TerminateJobObject(p.job, forcedExitCode)
	}
	p.mutex.Unlock()
	p.releaseConsole()
	return terminateErr
}

// releaseConsole は、擬似コンソールの所有権を一度だけ取り、別の goroutine で閉じる。
func (p *windowsProcess) releaseConsole() {
	p.mutex.Lock()
	if p.consoleReleased {
		p.mutex.Unlock()
		return
	}
	p.consoleReleased = true
	console := p.console
	p.mutex.Unlock()
	go windows.ClosePseudoConsole(console)
}

// Close は呼び出し側のパイプ端、コンソール、Job Object を解放する。
func (p *windowsProcess) Close() error {
	inputErr := p.input.Close()
	outputErr := p.output.Close()
	p.releaseConsole()
	p.mutex.Lock()
	var jobErr error
	if !p.jobReleased {
		p.jobReleased = true
		jobErr = windows.CloseHandle(p.job)
	}
	p.mutex.Unlock()
	return errors.Join(inputErr, outputErr, jobErr)
}

// Wait は木の終わりを待ち、その理由を返す。
func (p *windowsProcess) Wait() ExitInfo {
	<-p.exited
	return p.exit
}

// watch はプロセスツリーの終了を待ち、終了理由を記録して擬似コンソールを解放する。
func (p *windowsProcess) watch() {
	defer close(p.exited)
	info := ExitInfo{At: time.Now()}
	if _, err := windows.WaitForSingleObject(p.process, windows.INFINITE); err != nil {
		info.Code = -1
	} else {
		var code uint32
		if err := windows.GetExitCodeProcess(p.process, &code); err != nil {
			info.Code = -1
		} else {
			info.Code = int(code)
		}
	}
	if p.wasForced() {
		info.Code = -1
		info.Signal = "killed"
	}
	p.exit = info
	windows.CloseHandle(p.process)
	p.releaseConsole()
}
