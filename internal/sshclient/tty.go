package sshclient

import (
	"context"
	"io"
	"os"
	"os/signal"
	"sync"

	"golang.org/x/term"

	"sshc/internal/terminal"
)

// DefaultLocalSize は、端末の大きさを問い合わせられないときに使う値である。
//
// パイプの中で走っているときがそれである。80x24 は、それでも読める既定として
// 長く使われてきた寸法であり、ここで別の数を選ぶ理由が無い。
var DefaultLocalSize = terminal.Size{Cols: 80, Rows: 24}

// Attach は、このプロセスの端末を SSH のセッションへ繋ぐ。
//
// 戻るのはセッションが終わったときで、返すのはリモートの終了コードである。
//
// **端末の状態は必ず元に戻す。** 戻さないと、このコマンドが終わったあとの
// シェルがエコーも改行も失ったままになる。だから復元は defer に置く——
// 途中でどう抜けても通る道に置くしかない。
func Attach(ctx context.Context, process terminal.Process, in *os.File, out io.Writer) (int, error) {
	size := DefaultLocalSize
	descriptor := int(in.Fd())

	// テレタイプでなければ raw にしない。**大きさも問い合わせない。**
	// パイプの中で走っているときがそれで、その場合でも読み書きは通る。
	if term.IsTerminal(descriptor) {
		state, err := term.MakeRaw(descriptor)
		if err != nil {
			return 0, err
		}
		defer func() { _ = term.Restore(descriptor, state) }()

		if cols, rows, err := term.GetSize(descriptor); err == nil {
			size = terminal.Size{Cols: uint16(cols), Rows: uint16(rows)}
		}
		stop := watchResize(descriptor, process)
		defer stop()
	}
	_ = process.Resize(size)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var once sync.Once
	finish := func() { once.Do(func() { _ = process.Close() }) }

	// 読み手はこの goroutine を出ない。os.Stdin の Read は閉じても返らないので、
	// 待てば終わらない——セッションが終わったら、こちらは置いていく。
	go func() {
		_, _ = io.Copy(process, in)
	}()
	go func() {
		<-ctx.Done()
		finish()
	}()

	_, copyErr := io.Copy(out, process)
	info := process.Wait()
	finish()
	if copyErr != nil && info.Code == 0 {
		return 0, copyErr
	}
	return info.Code, nil
}

// watchResize は、端末の大きさが変わるたびにそれを送り直す。
//
// SIGWINCH を見ないと、ウィンドウを広げた瞬間から向こうのプログラムが
// 古い寸法で描き続ける。
func watchResize(descriptor int, process terminal.Process) func() {
	changed := make(chan os.Signal, 1)
	notifyResize(changed)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case <-changed:
				cols, rows, err := term.GetSize(descriptor)
				if err != nil {
					continue
				}
				_ = process.Resize(terminal.Size{Cols: uint16(cols), Rows: uint16(rows)})
			}
		}
	}()
	return func() {
		close(done)
		signal.Stop(changed)
	}
}
