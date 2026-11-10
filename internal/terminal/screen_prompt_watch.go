package terminal

import (
	"strings"
	"time"

	"sshc/third_party/vt"
)

// Claude Code は、ターミナルが問い合わせに答えたときだけ、OSC 7501 で問いを報告する。tmux は問い合わせを
// 自分のところで止め、そのあとに送られる DA1 に答えるので、tmux の下では何も報告されない。それでも問いは
// 画面に描かれる。代替画面が出ているあいだ（tmux ではいつも）、エンジンは自前のエミュレーターで出力を追い、
// 問いが画面に読めるあいだ blocked の記録を持つ（claude_code_prompt.go）。あとはその記録が、プログラムが
// 送った記録と同じように働く。セッションの一覧にも、通知にも出る。
//
// screenCheckpoint は使えない。あれは流し直しのバッファが手放したものしか読まないので、バッファ1つ分遅れる。
// 自分で報告するプログラムには任せる。通常の画面の出力には何もしないので、tmux の外の Claude Code は
// まったく読まない。

const (
	// screenPromptRecordID は、画面から読んだ問いを入れておく記録の ID である。App は空のままにする。
	// resumablePrograms がその名前を読むが、tmux の下のプログラムは、接続が切れてもそこで走り続けているので、
	// 起動し直す必要が無い。
	screenPromptRecordID = "sshc-screen-prompt"
	// screenPromptSettle は、画面を読む前に出力が止まっていなければならない長さである。描き直しを途中で
	// 読まないためである。tmux は描き直しを一気に書く。Claude Code 2.1.318 がいくつかの問いのあいだ続ける
	// 0.6秒おきの描き直しより短いので、それに読みを止められない。
	screenPromptSettle = 300 * time.Millisecond
	// screenPromptLongestWait は、その待ちの上限である。ほかのペインが休まず出力していても、問いを隠させない。
	screenPromptLongestWait = 2 * time.Second
	// screenPromptGoneAfter は、記録を捨てる前に、問いが画面から消えていなければならない長さである。大きさの
	// 変更や遅い回線では、描き直しが screenPromptSettle を過ぎても途中のままになることがあり、戻るたびに通知が
	// 出てしまう。
	screenPromptGoneAfter = time.Second
	// enterAlternateScreen は、出力が今入った画面について、新しいエミュレーターを始める。入るためのシーケンスは、
	// 前のかたまりで終わっていることがある。
	enterAlternateScreen = "\x1b[?1049h"
)

// screenPromptWatch は、1つのセッションが自分の画面を読んでいる状態である。Session.mutex が守る。
type screenPromptWatch struct {
	// emulator は、代替画面が出ているあいだ出力を追う。出ていなければ nil。
	emulator *vt.SnapshotEmulator
	// timer は、鳴ったときに画面を読む。
	timer *time.Timer
	// unreadSince は、まだ読んでいない最初の出力が来た時刻である。すべて読んでいればゼロ。
	unreadSince time.Time
	// goneSince は、問いが画面に無いと最初に分かった時刻である。問いがあるあいだはゼロ。
	goneSince time.Time
}

// stop は、追うのをやめる。すでに走り始めた読みは、エミュレーターが無いのを見て終わる。
func (w *screenPromptWatch) stop() {
	if w.timer != nil {
		w.timer.Stop()
	}
	*w = screenPromptWatch{}
}

// stopFollowingScreenIfPanicked は、Session.mutex を持ったまま、エミュレーターを使うところすべてで defer する。
// 出力は向こう側から来るので、エミュレーターが受け取れないシーケンスは、サーバーが送れるものである。誰にも
// 知らされない問いが1つ出ることは、エンジンがすべてのセッションと一緒に落ちることに比べれば小さい。
func (s *Session) stopFollowingScreenIfPanicked() {
	if recover() == nil {
		return
	}
	s.screenPrompt.stop()
	s.programStatus.clear(screenPromptRecordID)
	s.reportBlockedProgramLocked()
}

// resizeFollowedScreenLocked は、エミュレーターをセッションの大きさに保つ。
func (s *Session) resizeFollowedScreenLocked(size Size) {
	defer s.stopFollowingScreenIfPanicked()
	if s.screenPrompt.emulator != nil {
		s.screenPrompt.emulator.Resize(int(size.Cols), int(size.Rows))
	}
}

// newAlternateScreenObserver は、出力が代替画面に入ったことを知らせる。出たことはエミュレーターが見る。
func newAlternateScreenObserver(entered func()) *controlSequenceScanner {
	return &controlSequenceScanner{sequence: func(parameters string, final byte) {
		modes, private := strings.CutPrefix(parameters, "?")
		if !private || final != 'h' {
			return
		}
		for _, mode := range strings.Split(modes, ";") {
			if mode == "47" || mode == "1047" || mode == "1049" {
				entered()
				return
			}
		}
	}}
}

// startFollowingScreen は、出力が代替画面に入った世代で、エミュレーターを作る。
func (s *Session) startFollowingScreen(generation uint64) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	defer s.stopFollowingScreenIfPanicked()
	if generation != s.generation || s.exited != nil || s.screenPrompt.emulator != nil {
		return
	}
	s.screenPrompt.emulator = vt.NewSnapshotEmulator(int(s.size.Cols), int(s.size.Rows))
	_, _ = s.screenPrompt.emulator.WriteString(enterAlternateScreen)
}

// followScreenOutput は、プログラムの出力のかたまりをエミュレーターに渡す。
func (s *Session) followScreenOutput(generation uint64, chunk []byte) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	defer s.stopFollowingScreenIfPanicked()
	watch := &s.screenPrompt
	if generation != s.generation || watch.emulator == nil {
		return
	}
	_, _ = watch.emulator.Write(chunk)
	if !watch.emulator.AltScreenShown() {
		// プログラムが出たか、tmux が離れた。問いを読む画面はもう無い。
		watch.stop()
		s.programStatus.clear(screenPromptRecordID)
		s.reportBlockedProgramLocked()
		return
	}
	now := time.Now()
	if watch.unreadSince.IsZero() {
		watch.unreadSince = now
	}
	wait := min(screenPromptSettle, screenPromptLongestWait-now.Sub(watch.unreadSince))
	if watch.timer == nil {
		watch.timer = time.AfterFunc(wait, s.readScreenPrompt)
	} else {
		watch.timer.Reset(wait)
	}
}

// readScreenPrompt は、記録を、画面が言っているとおりにする。
func (s *Session) readScreenPrompt() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	defer s.stopFollowingScreenIfPanicked()
	watch := &s.screenPrompt
	if watch.emulator == nil {
		return
	}
	watch.unreadSince = time.Time{}
	if s.programStatus.hasLiveReport() {
		s.programStatus.clear(screenPromptRecordID)
	} else if kind, asking := claudeCodePromptOnScreen(watch.emulator.ShownCells()); asking {
		watch.goneSince = time.Time{}
		s.keepScreenPromptRecordLocked(kind)
	} else {
		s.dropScreenPromptRecordOnceGoneLocked()
	}
	// 再接続のあとにも呼ぶ。再接続は、何も知らせずに記録を捨てる（reportBlockedProgramLocked）。
	s.reportBlockedProgramLocked()
}

// keepScreenPromptRecordLocked は、記録がすでにそう言っていなければ、記録を入れる。新しい報告は新しい Version を
// 持つので、入れ直すと、まだ見ていない問いとして数えられてしまう。
func (s *Session) keepScreenPromptRecordLocked(kind ProgramStatusKind) {
	if s.programStatus.byID[screenPromptRecordID].Kind == kind {
		return
	}
	s.programStatus.apply(programStatusReport{
		id:     screenPromptRecordID,
		status: ProgramStatus{State: ProgramStatusBlocked, Kind: kind},
	})
}

// dropScreenPromptRecordOnceGoneLocked は、問いが画面から消えたままなら記録を捨てる。そうでなければ、その
// 時刻にもう一度見る。本当に消えた問いは、それ以上出力を出さないかもしれないためである。
func (s *Session) dropScreenPromptRecordOnceGoneLocked() {
	watch := &s.screenPrompt
	if _, kept := s.programStatus.byID[screenPromptRecordID]; !kept {
		return
	}
	now := time.Now()
	if watch.goneSince.IsZero() {
		watch.goneSince = now
	}
	if remaining := screenPromptGoneAfter - now.Sub(watch.goneSince); remaining > 0 {
		watch.timer.Reset(remaining)
		return
	}
	watch.goneSince = time.Time{}
	s.programStatus.clear(screenPromptRecordID)
}
