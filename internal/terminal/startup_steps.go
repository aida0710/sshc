package terminal

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// 接続時の自動実行は、シェルが使える状態になるたびに（再接続を含む）、手順を順に打ち込む。待つ文字列の
// ある手順は、シェルの出力にそれが出てから打ち込む（sudo や enable の問いを待ってから答えるため）。

// StartupStep is one step typed into a shell once it is ready. When Wait is not empty, the step first waits for it in
// the shell's own output; then it types Send, and Enter after it when Enter is set.
type StartupStep struct {
	Wait  string
	Send  string
	Enter bool
	// Optional steps are passed over when their text does not come in time. Any other step stops the steps there.
	Optional bool
}

// payload is what the step types.
func (step StartupStep) payload() []byte {
	if step.Enter {
		return []byte(step.Send + "\r")
	}
	return []byte(step.Send)
}

// ShellOutput is a Process that can hand over its shell's own output apart from what sshc writes into the terminal
// around it (authentication prompts and [sshc] lines). Startup steps that wait for a text look only at that output, so
// they never answer an authentication prompt.
type ShellOutput interface {
	// WatchShellOutput hands seen the end of what the shell wrote before the call, then each piece it writes after, in
	// order, until stop is called. seen runs while the shell's output waits for it, so it must only take the bytes.
	WatchShellOutput(seen func([]byte)) (stop func())
}

// startupStepTimedOutNotice tells, in the terminal, that the startup steps stopped because a text did not come.
const startupStepTimedOutNotice = "接続時の自動実行を止めました。「%s」が%d秒のうちに表示されませんでした。"

// startupStepsUnwatchableNotice tells that the steps could not run, for a process whose shell output cannot be told
// apart from sshc's own lines.
const startupStepsUnwatchableNotice = "この接続ではシェルの出力を待てないため、接続時の自動実行の手順を送りませんでした。"

// maxWatchedShellOutput is how much of what the shell wrote since the last step is kept to look for the next step's
// text in. The text is a prompt, written last.
const maxWatchedShellOutput = 64 << 10

// startupStepsWait reports whether any step waits for a text.
func startupStepsWait(steps []StartupStep) bool {
	for _, step := range steps {
		if step.Wait != "" {
			return true
		}
	}
	return false
}

// runStartupSteps types the steps of startup into process one after another, waiting first for the text of a step
// that has one. It stops when the steps are done, when a step's text does not come in time, or when process is no
// longer the session's (a reconnect replaced it, or the session ended).
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
		// What the shell wrote before this step does not answer the next one.
		watched.forget()
		_, _ = process.Write(step.payload())
	}
}

// isGeneration reports whether generation is still the session's process, and the session has not stopped.
func (s *Session) isGeneration(generation uint64) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.generation == generation && !s.stoppedLocked() && s.exited == nil
}

// announceStartup writes message as an [sshc] line, while generation is still the session's. The shell has usually
// drawn its prompt by then, so the line starts on a line of its own, as the reconnect lines do.
func (s *Session) announceStartup(generation uint64, message string) {
	if s.isGeneration(generation) {
		s.publish([]byte("\r\n[sshc] " + message + "\r\n"))
	}
}

// watchedShellOutput holds what the shell wrote since the last step was typed, for the next step to look in.
type watchedShellOutput struct {
	mutex   sync.Mutex
	written []byte
	// changed is signalled after each add; a pending signal is enough for any number of adds.
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

// shows reports whether text is in what the shell wrote, read as the terminal shows it: without colours and other
// control sequences.
func (w *watchedShellOutput) shows(text string) bool {
	w.mutex.Lock()
	written := append([]byte(nil), w.written...)
	w.mutex.Unlock()
	return strings.Contains(PlainTextFrom(written, 0), text)
}

// waitFor waits until the shell has written text, for at most limit, and reports whether it did. A session that
// stops (stopping) or ends (done) ends the wait at once.
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
