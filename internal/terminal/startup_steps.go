package terminal

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// 接続時の自動実行は、シェルが使える状態になるたびに（再接続を含む）、手順を順に打ち込む。待つ文字列の
// ある手順は、シェルの出力にそれが出てから打ち込む（sudo や enable の問いを待ってから答えるため）。

// StartupStep は、シェルが使えるようになったら打ち込む手順1つである。Wait が空でなければ、まずシェル自身の
// 出力にそれが出るのを待つ。それから Send を打ち、Enter が立っていれば続けて Enter を打つ。
type StartupStep struct {
	Wait  string
	Send  string
	Enter bool
	// Optional の手順は、待つ文字が時間内に出なければ飛ばす。それ以外の手順では、そこで手順を止める。
	Optional bool
}

// payload は、この手順が打ち込むものである。
func (step StartupStep) payload() []byte {
	if step.Enter {
		return []byte(step.Send + "\r")
	}
	return []byte(step.Send)
}

// ShellOutput は、シェル自身の出力を、sshc がそのまわりにターミナルへ書くもの（認証の問いと [sshc] の行）と
// 分けて渡せる Process である。文字を待つ起動時の手順は、その出力だけを見るので、認証の問いに答えることが
// 無い。
type ShellOutput interface {
	// WatchShellOutput は、呼ばれる前にシェルが書いたものの終わりを seen に渡し、そのあとは、stop が呼ばれるまで、
	// 書かれたものを順に1つずつ渡す。seen が走っているあいだシェルの出力は待たされるので、seen はバイトを
	// 受け取るだけにする。
	WatchShellOutput(seen func([]byte)) (stop func())
}

// startupStepTimedOutNotice は、待つ文字が出なかったために起動時の手順を止めたことを、ターミナルで伝える。
const startupStepTimedOutNotice = "接続時の自動実行を止めました。「%s」が%d秒のうちに表示されませんでした。"

// startupStepsUnwatchableNotice は、手順を実行できなかったことを伝える。シェルの出力を sshc 自身の行と
// 見分けられない process のときに出す。
const startupStepsUnwatchableNotice = "この接続ではシェルの出力を待てないため、接続時の自動実行の手順を送りませんでした。"

// maxWatchedShellOutput は、次の手順の文字を探すために、最後の手順からあとにシェルが書いたものをどれだけ
// 持つかである。待つ文字はプロンプトで、最後に書かれる。
const maxWatchedShellOutput = 64 << 10

// startupStepsWait は、文字を待つ手順が1つでもあるかを返す。
func startupStepsWait(steps []StartupStep) bool {
	for _, step := range steps {
		if step.Wait != "" {
			return true
		}
	}
	return false
}

// runStartupSteps は、startup の手順を、process へ1つずつ打ち込む。待つ文字のある手順は、まずそれを待つ。
// 手順が終わったとき、手順の文字が時間内に出なかったとき、process がもうセッションのものでないとき
// （再接続が入れ替えたか、セッションが終わった）に止まる。
func (s *Session) runStartupSteps(process Process, generation uint64, startup Startup) {
	shell, ok := process.(ShellOutput)
	if !ok {
		s.announceStartup(generation, startupStepsUnwatchableNotice)
		return
	}
	watched := newWatchedShellOutput()
	stop := shell.WatchShellOutput(watched.add)
	defer stop()
	for _, step := range startup.Steps {
		if step.Wait != "" {
			found := watched.waitFor(step.Wait, startup.Wait, s.stopping, s.done)
			if !s.isGeneration(generation) {
				return
			}
			if !found {
				if step.Optional {
					continue
				}
				s.announceStartup(generation, fmt.Sprintf(startupStepTimedOutNotice, step.Wait, int(startup.Wait/time.Second)))
				return
			}
		}
		if !s.isGeneration(generation) {
			return
		}
		// この手順の前にシェルが書いたものは、次の手順の答えにならない。
		watched.forget()
		_, _ = process.Write(step.payload())
	}
}

// isGeneration は、generation がまだセッションの process で、セッションが止まっていないかを返す。
func (s *Session) isGeneration(generation uint64) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.generation == generation && !s.stoppedLocked() && s.exited == nil
}

// announceStartup は、generation がまだセッションのものであるあいだ、message を [sshc] の行として書く。
// そのころにはシェルがプロンプトを描いていることが多いので、再接続の行と同じく、行を改めてから書く。
func (s *Session) announceStartup(generation uint64, message string) {
	if s.isGeneration(generation) {
		s.publish([]byte("\r\n[sshc] " + message + "\r\n"))
	}
}

// watchedShellOutput は、最後の手順を打ち込んでからシェルが書いたものを持つ。次の手順がその中を探す。
type watchedShellOutput struct {
	mutex   sync.Mutex
	written []byte
	// changed は、足すたびに合図を受ける。何度足しても、待っている合図が1つあれば足りる。
	changed chan struct{}
}

func newWatchedShellOutput() *watchedShellOutput {
	return &watchedShellOutput{changed: make(chan struct{}, 1)}
}

func (w *watchedShellOutput) add(p []byte) {
	w.mutex.Lock()
	w.written = append(w.written, p...)
	if excess := len(w.written) - maxWatchedShellOutput; excess > 0 {
		w.written = append(w.written[:0], w.written[excess:]...)
	}
	w.mutex.Unlock()
	select {
	case w.changed <- struct{}{}:
	default:
	}
}

func (w *watchedShellOutput) forget() {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.written = w.written[:0]
}

// shows は、シェルが書いたものの中に text があるかを返す。ターミナルに出る形（色などの制御シーケンスを
// 除いた形）で読む。
func (w *watchedShellOutput) shows(text string) bool {
	w.mutex.Lock()
	written := append([]byte(nil), w.written...)
	w.mutex.Unlock()
	return strings.Contains(PlainTextFrom(written, 0), text)
}

// waitFor は、シェルが text を書くまで、長くても limit だけ待ち、書いたかを返す。セッションが止まる
// （stopping）か終わる（done）と、待ちはすぐに終わる。
func (w *watchedShellOutput) waitFor(text string, limit time.Duration, stopping, done <-chan struct{}) bool {
	timer := time.NewTimer(limit)
	defer timer.Stop()
	for {
		if w.shows(text) {
			return true
		}
		select {
		case <-w.changed:
		case <-timer.C:
			return false
		case <-stopping:
			return false
		case <-done:
			return false
		}
	}
}
