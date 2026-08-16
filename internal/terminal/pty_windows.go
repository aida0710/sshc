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
//
// **これで強制終了を見分けるのではない。** 子が自分で同じ値を返すことはありうる。
// 見分けるのは、この実装が強制したという事実そのものである。
const forcedExitCode = 1

// conpty は、CreatePseudoConsole の在否を呼ぶ前に確かめるための探りである。
//
// **x/sys の CreatePseudoConsole は、export が無いと panic する。** LazyProc が
// mustFind を通るからで、ERROR_PROC_NOT_FOUND が返ることはない。Windows 10 1809
// より前だと伝える方法は、呼ぶ前に Find で調べる以外に無い。手で解決した proc を
// 呼ぶことはせず、ここは在否を調べるためだけに使う。
var conpty = windows.NewLazySystemDLL("kernel32.dll").NewProc("CreatePseudoConsole")

// ErrPseudoConsoleUnavailable は、この Windows が擬似コンソールを持たないことを言う。
var ErrPseudoConsoleUnavailable = errors.New(
	"this version of Windows has no pseudoconsole; sshc needs Windows 10 version 1809 or later")

// WindowsStarter は、擬似コンソールを確保する唯一の実装である。
type WindowsStarter struct{}

func NewStarter() Starter { return WindowsStarter{} }

func (WindowsStarter) Start(ctx context.Context, command Command, size Size) (Process, error) {
	if err := conpty.Find(); err != nil {
		return nil, ErrPseudoConsoleUnavailable
	}
	if !size.Valid() {
		return nil, ErrInvalidSize
	}
	// **PATH を見ない。** このアプリケーションが起こすものは、呼び出し側が
	// 絶対パスで名指ししたものだけである。
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
//
// どの段で失敗しても、そこまでに作ったものだけを正確に手放す。**途中まで
// 組み立てたものを返さない。**
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
	// **擬似コンソール側の端は、ここで手放す。** Microsoft がそう要求している。
	//
	// ただしこれだけでは読み取りは終わらない。出力パイプの書き手は擬似コンソール
	// そのものでもあり、EOF が来るのはそれを閉じたときである——それを行うのは
	// watch である。ここで手放すのは、こちらが余計な書き手を残さないためである。
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
		// **こちら側の端を先に手放し、コンソールは別の goroutine で閉じる。**
		// ClosePseudoConsole は 24H2 より前では際限なく待つことがあり、この
		// 巻き戻しは HTTP のハンドラの上で走っている——実行できないプログラムを
		// 断るだけで、ハンドラが返らなくなる。
		windows.CloseHandle(inputWrite)
		windows.CloseHandle(outputRead)
		go windows.ClosePseudoConsole(console)
	}()

	attributes, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return nil, fmt.Errorf("terminal: allocate the process attributes: %w", err)
	}
	defer attributes.Delete()
	// vet が受け取る綴りで書く。unsafe.Pointer(console) は実行時には同じ値だが
	// vet はそれを不正な変換として拒み、Windows の CI は vet を走らせる。
	if err := attributes.Update(
		windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
		*(*unsafe.Pointer)(unsafe.Pointer(&console)),
		unsafe.Sizeof(console),
	); err != nil {
		return nil, fmt.Errorf("terminal: attach the pseudoconsole: %w", err)
	}

	startup := windows.StartupInfoEx{ProcThreadAttributeList: attributes.List()}
	// **StartupInfoEx 全体の大きさである。** 埋め込まれた StartupInfo の
	// 大きさを渡すと、CreateProcess は属性リストを無視し、子はコンソールを
	// 一つも受け取らない。
	startup.Cb = uint32(unsafe.Sizeof(startup))

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

	// **中断状態で起こす。** ジョブへ入れ終える前に走らせると、その隙に子が
	// 起こした孫はジョブの外に生まれ、あとから畳めなくなる。
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_SUSPENDED)
	if environment != nil {
		// これが無いと、カーネルは UTF-16 の環境ブロックを ANSI として読む。
		flags |= windows.CREATE_UNICODE_ENVIRONMENT
	}
	var information windows.ProcessInformation
	if err := windows.CreateProcess(
		applicationName, commandLine, nil, nil,
		// **何も継がせない。** 子がコンソールを受け取る道は属性リストだけである。
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

// assignToJob は、木ごと畳めるようにするジョブを作って子を入れる。
func assignToJob(process windows.Handle) (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, fmt.Errorf("terminal: create the job object: %w", err)
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	// SetInformationJobObject は (ret, err) を返し、**ret == 0 が失敗である。**
	if ret, err := windows.SetInformationJobObject(
		job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)),
	); ret == 0 {
		windows.CloseHandle(job)
		return 0, fmt.Errorf("terminal: limit the job object: %w", err)
	}
	// 入れられないことは致命である。**回避しない。** 入れ子のジョブは Windows 8
	// 以降で使えるので、Electron や CI エージェントのジョブの中に居ても入る。
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		windows.CloseHandle(job)
		return 0, fmt.Errorf("terminal: put the console process in its job: %w", err)
	}
	return job, nil
}

// composeCommandLine は、argv をひとつのコマンドラインへ畳む。
//
// 手で連結しない。引数がコマンドラインの意味を変えられないようにするのは、
// この引用規則そのものである。
func composeCommandLine(command Command) string {
	argv0 := command.Argv0
	if argv0 == "" {
		argv0 = command.Path
	}
	return windows.ComposeCommandLine(append([]string{argv0}, command.Arguments...))
}

// environmentBlock は、UTF-16 の環境ブロックを組み立てる。
//
// nil の Env はこのプロセスの環境の継承を意味するので、nil を返す。
//
// **NUL を含む項目と、`=` を持たない項目は拒む。** どちらもブロックを途中で
// 終わらせられるので、一項目で子の環境全体を差し替えられてしまう。
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
	// 各項目は UTF16FromString が NUL で終えている。最後にもう一つ足して
	// 二重の NUL にする。**空の環境も同じ形でなければならない**ので、項目が
	// 一つも無いときは二つ足す——一つだけでは終端になっていない。
	if len(builder) == 0 {
		builder = append(builder, 0)
	}
	builder = append(builder, 0)
	return &builder[0], nil
}

// windowsProcess は、擬似コンソールひとつと、その中の木である。
//
// **console と job は生のハンドルである。** os.File と違って参照が数えられず、
// Windows はハンドルの値を使い回すので、手放したあとに使うと失敗せずに無関係な
// ものへ届く。だから「一度だけ閉じる」ことと「閉じたあとは使わない」ことを、
// ひとつの錠で同じ規則にしてある。
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

	// exited は、見張りが終了理由を書き終えたことを示す。
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

// Resize は、まだ生きているコンソールにだけ届く。
func (p *windowsProcess) Resize(size Size) error {
	if !size.Valid() {
		return ErrInvalidSize
	}
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	if p.consoleReleased {
		// もう畳まれている。大きさを伝える相手が居ない。
		return nil
	}
	return windows.ResizePseudoConsole(p.console, windows.Coord{X: int16(size.Cols), Y: int16(size.Rows)})
}

// Hangup は、木に終わってほしいという意思である。
//
// 擬似コンソールを閉じることが、繋がっているコンソールアプリケーションに
// 「もう端末が無い」と伝える手段である。**何も強制終了しない。**
//
// 閉じる呼び出しそのものは別の goroutine で行い、ここは即座に返る。
// ClosePseudoConsole は Windows 11 24H2 より前では際限なく待つことがあり、
// これは HTTP のハンドラからそのまま呼ばれる。
func (p *windowsProcess) Hangup() error {
	p.releaseConsole()
	return nil
}

// ForceClose は、締切に達した合図である。**待たない。**
func (p *windowsProcess) ForceClose() error {
	p.mutex.Lock()
	p.forced = true
	var terminateErr error
	if !p.jobReleased {
		// ジョブを畳めば木ごと終わる。コンソールに繋がっていない孫も含めてである。
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

// Close は、この側の端とコンソールとジョブを手放す。
//
// **プロセスのハンドルはここでは閉じない。** Registry は遅れて返ってきた
// Process に対して ForceClose、Close、Wait をこの順で呼ぶ。ここで閉じると、
// 続く Wait は無効なハンドルで即座に戻って作り話の終了理由を報告し、しかも
// Windows はハンドルの値を使い回すので無関係なものを待つことすらある。
func (p *windowsProcess) Close() error {
	inputErr := p.input.Close()
	outputErr := p.output.Close()
	p.releaseConsole()
	p.mutex.Lock()
	var jobErr error
	if !p.jobReleased {
		p.jobReleased = true
		// **ジョブは最後である。** これを閉じることが、コンソールに繋がって
		// いない孫を取り除く。
		jobErr = windows.CloseHandle(p.job)
	}
	p.mutex.Unlock()
	return errors.Join(inputErr, outputErr, jobErr)
}

// Wait は木の終わりを待ち、その理由を返す。
//
// プロセスのハンドルを閉じるのはここだけであり、終了コードを読み終えたあとである。
func (p *windowsProcess) Wait() ExitInfo {
	<-p.exited
	return p.exit
}

// watch は、木が終わるのを待ち、終了理由を記録して擬似コンソールを手放す。
//
// **子が終わっただけでは、読み取りは終わらない。** 出力パイプの書き手は子では
// なく擬似コンソールそのものであり、それが開いている限り EOF は来ない。ここで
// 閉じなければ、利用者が exit と打っただけのセッションは永久に「生きている」
// ままになり、pump は done を閉じず、engine lock も手放されない。
//
// **Unix には無い段である。** 向こうは子が死ねば PTY の従側が閉じ、読み取りが
// そこで終わる。ConPTY はそうならない。
//
// プロセスのハンドルを閉じるのもここだけである。所有者を一つにしておかないと、
// Wait と見張りが同じハンドルを取り合う。
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
		// **終了コードでは見分けられない。** TerminateJobObject は与えた値を
		// 木のすべてに刻むので、子が同じ値を返した場合と区別がつかない。
		// 見分けるのは、この実装が強制したという事実である。Unix が合図で
		// 終わった子を報告するのと同じ形にする。
		info.Code = -1
		info.Signal = "killed"
	}
	p.exit = info
	windows.CloseHandle(p.process)
	p.releaseConsole()
}
