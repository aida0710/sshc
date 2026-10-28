//go:build windows

package main

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// 裏で動くエンジンは、起動したコンソールを手放す（releaseEngineConsole）。コンソールの
// 無いプロセスには、Windows の終了・サインアウトの知らせ（CTRL_SHUTDOWN_EVENT など）が
// 届かず、そのまま終わらされる。そのとき開いていたセッションの記録（開き直しの申し出）を
// 書けない。コンソールの無いプロセスが終了を知る道は、トップレベルのウィンドウに配られる
// WM_QUERYENDSESSION・WM_ENDSESSION だけなので、見えないウィンドウを1つ持つ。
//
// user32 を読み込むと、コンソールのあるプロセスではコンソールの終了の知らせが届かなく
// なる。コンソールを手放した裏のエンジンだけがこれを使う。

// sessionEndWait は、WM_ENDSESSION を受けてから、エンジンが止まり終える（記録を書く）のを
// 待つ上限である。WM_ENDSESSION から戻ると、プロセスはいつ終わらされてもよい。止まるのは
// ふつう1秒かからず、長く待たせると Windows が「終了を妨げているアプリ」として出す。
const sessionEndWait = 10 * time.Second

// x/sys/windows に無いウィンドウのメッセージの番号（winuser.h）。
const (
	windowMessageDestroy         = 0x0002
	windowMessageClose           = 0x0010
	windowMessageQueryEndSession = 0x0011
	windowMessageEndSession      = 0x0016
)

const (
	sessionEndWindowClassName = "sshcEngineSessionEnd"
	// allowSessionEnd は、WM_QUERYENDSESSION に「終わってよい」と答える値である。
	allowSessionEnd = 1
)

var (
	user32                = windows.NewLazySystemDLL("user32.dll")
	procRegisterClassExW  = user32.NewProc("RegisterClassExW")
	procCreateWindowExW   = user32.NewProc("CreateWindowExW")
	procDefWindowProcW    = user32.NewProc("DefWindowProcW")
	procGetMessageW       = user32.NewProc("GetMessageW")
	procDispatchMessageW  = user32.NewProc("DispatchMessageW")
	procPostMessageW      = user32.NewProc("PostMessageW")
	procPostQuitMessage   = user32.NewProc("PostQuitMessage")
	sessionEndingCallback = windows.NewCallback(sessionEndWindowProcedure)
	// sessionEnding は、WM_ENDSESSION を受けたときに呼ぶ。エンジンは1つのプロセスに1つ
	// なので、ウィンドウの手続きからはこれを引く。
	sessionEnding atomic.Pointer[func()]
)

// windowClass は WNDCLASSEXW である。
type windowClass struct {
	size       uint32
	style      uint32
	procedure  uintptr
	classExtra int32
	extra      int32
	instance   windows.Handle
	icon       windows.Handle
	cursor     windows.Handle
	background windows.Handle
	menuName   *uint16
	className  *uint16
	smallIcon  windows.Handle
}

// windowMessage は MSG である。
type windowMessage struct {
	window  uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	point   struct{ x, y int32 }
	private uint32
}

// sessionEndWatch は、見えないウィンドウと、それが運ぶ止まる理由である。
type sessionEndWatch struct {
	ctx context.Context
	// window は見えないウィンドウ。作れなかったときは 0 で、そのときは今までどおり
	// 終了を知らずに終わらされる。
	window uintptr
	// finish は、エンジンが止まり終えたときに呼ぶ。待っている WM_ENDSESSION を返し、
	// ウィンドウを閉じる。
	finish func()
}

// watchSessionEnd は、Windows の終了・サインアウトで ctx を errTerminated で止める。
// 返す finish は、エンジンが止まり終えてから呼ぶ。
func watchSessionEnd(ctx context.Context) (context.Context, func()) {
	watch := startSessionEndWatch(ctx)
	return watch.ctx, watch.finish
}

func startSessionEndWatch(ctx context.Context) sessionEndWatch {
	sessionCtx, cancel := context.WithCancelCause(ctx)
	stopped := make(chan struct{})
	ending := func() {
		cancel(errTerminated)
		select {
		case <-stopped:
		case <-time.After(sessionEndWait):
		}
	}
	sessionEnding.Store(&ending)
	created := make(chan uintptr, 1)
	go runSessionEndWindow(created)
	window := <-created
	var once sync.Once
	return sessionEndWatch{ctx: sessionCtx, window: window, finish: func() {
		once.Do(func() {
			close(stopped)
			if window != 0 {
				procPostMessageW.Call(window, windowMessageClose, 0, 0)
			}
			cancel(nil)
		})
	}}
}

// runSessionEndWindow は、見えないウィンドウを作って、閉じるまでメッセージを配る。
// ウィンドウはそれを作ったスレッドにしかメッセージが来ないので、スレッドを固定する。
func runSessionEndWindow(created chan<- uintptr) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	window := createSessionEndWindow()
	created <- window
	if window == 0 {
		return
	}
	var message windowMessage
	for {
		result, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if int32(result) <= 0 {
			return
		}
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&message)))
	}
}

func createSessionEndWindow() uintptr {
	className, err := windows.UTF16PtrFromString(sessionEndWindowClassName)
	if err != nil {
		return 0
	}
	var instance windows.Handle
	if err := windows.GetModuleHandleEx(0, nil, &instance); err != nil {
		return 0
	}
	class := windowClass{procedure: sessionEndingCallback, instance: instance, className: className}
	class.size = uint32(unsafe.Sizeof(class))
	// 同じプロセスで2度目に登録すると失敗するが、登録済みのクラスでウィンドウは作れる。
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&class)))
	// WS_OVERLAPPED で、表示はしない。HWND_MESSAGE の窓には終了の知らせが配られない。
	// 位置と大きさは使わないので 0 にする。
	window, _, _ := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(className)), 0, 0, 0, 0, 0, 0, 0, 0, uintptr(instance), 0)
	runtime.KeepAlive(className)
	return window
}

func sessionEndWindowProcedure(window, message, wParam, lParam uintptr) uintptr {
	switch message {
	case windowMessageQueryEndSession:
		return allowSessionEnd
	case windowMessageEndSession:
		// wParam が 0 なら、終了は取りやめになった。
		if wParam != 0 {
			if ending := sessionEnding.Load(); ending != nil {
				(*ending)()
			}
		}
		return 0
	case windowMessageDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	result, _, _ := procDefWindowProcW.Call(window, message, wParam, lParam)
	return result
}
