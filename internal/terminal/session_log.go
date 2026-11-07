package terminal

import (
	"fmt"
	"time"
)

// セッションのログは、ターミナルに出たものを文字でファイルへ書き続ける（internal/sessionlog）。書くのは、
// 接続が使える状態になったあとの出力だけである。SSH のセッションでは、シェル自身の出力だけを書く
// （ShellOutput）。認証の問いと、そこへ打った答えは書かない。

// OutputLog keeps what a session's terminal showed (internal/sessionlog.Log). Its methods return at once; the log
// writes on its own.
type OutputLog interface {
	Write(output []byte)
	Resize(size Size)
	// Note writes a line that says when something happened to the session.
	Note(text string)
	// Hide keeps secrets out of what the log writes from now on.
	Hide(secrets []string)
	// Close writes what is left and closes the log.
	Close()
}

// LogTarget is the session a log is opened for (Registry.OpenLog).
type LogTarget struct {
	ID   string
	Kind Kind
	// Alias is the connection name, or the destination a session opened without one; empty for a local shell.
	Alias   string
	Started time.Time
	Size    Size
}

const (
	logConnectedNote    = "接続しました。"
	logShellOpenedNote  = "ローカルシェルを開きました。"
	logReconnectedNote  = "再接続しました。"
	logDisconnectedNote = "接続が切れました。"
	logEndedNote        = "セッションが終わりました。"
)

// logOpenFailedNotice tells, in the terminal, that the session keeps no log because its file could not be made.
const logOpenFailedNotice = "\r\n[sshc] セッションのログを開けませんでした（%v）。このセッションのログは保存しません。\r\n"

// openLog opens the session's log as the session opens, when the settings keep one for it.
func (s *Session) openLog(open func(LogTarget) (OutputLog, error)) {
	if open == nil {
		return
	}
	log, err := open(LogTarget{ID: s.id, Kind: s.kind, Alias: s.alias, Started: s.started, Size: s.size})
	if err != nil {
		s.publish([]byte(fmt.Sprintf(logOpenFailedNotice, err)))
		return
	}
	s.log = log
}

// startLog follows, in the session's log, the output of process, ready now as the session's generation generation.
// When process can hand over its shell's own output (an SSH session), only that reaches the log, beginning with what
// the shell wrote before it was ready; otherwise (a local shell, with no authentication to keep out) everything it
// prints.
func (s *Session) startLog(process Process, generation uint64) {
	s.mutex.Lock()
	log := s.log
	if log == nil || s.generation != generation {
		s.mutex.Unlock()
		return
	}
	note := logConnectedNote
	switch {
	case s.logConnectedBefore:
		note = logReconnectedNote
	case s.kind == KindShell:
		note = logShellOpenedNote
	}
	s.logConnectedBefore = true
	shell, watchable := process.(ShellOutput)
	if !watchable {
		s.logPumpGeneration = generation
	}
	s.mutex.Unlock()
	log.Note(note)
	if !watchable {
		return
	}
	stop := shell.WatchShellOutput(log.Write)
	s.mutex.Lock()
	if s.generation != generation {
		s.mutex.Unlock()
		stop()
		return
	}
	s.stopLogWatch = stop
	s.mutex.Unlock()
}

// logPumpOutput writes output that process generation printed to the log, while the log follows that process's
// output itself (startLog).
func (s *Session) logPumpOutput(generation uint64, output []byte) {
	s.mutex.Lock()
	log := s.log
	following := s.logPumpGeneration == generation
	s.mutex.Unlock()
	if log != nil && following {
		log.Write(output)
	}
}

// endLogOfProcess stops following a process that ended, and says so in the log when an SSH connection that had
// connected was lost.
func (s *Session) endLogOfProcess(connected bool) {
	s.mutex.Lock()
	log, stop := s.log, s.stopLogWatch
	connected = connected && s.kind == KindSSH
	s.stopLogWatch = nil
	s.logPumpGeneration = 0
	s.mutex.Unlock()
	if stop != nil {
		stop()
	}
	if log != nil && connected {
		log.Note(logDisconnectedNote)
	}
}

// hideInLog keeps secrets that sshc types into the shell out of the session's log, where the shell may echo them.
func (s *Session) hideInLog(secrets []string) {
	s.mutex.Lock()
	log := s.log
	s.mutex.Unlock()
	if log != nil && len(secrets) > 0 {
		log.Hide(secrets)
	}
}

// HideInLog keeps secrets out of the session's log: a command typed into the terminal (a quick command with a secret
// variable) is echoed by the shell.
func (s *Session) HideInLog(secrets []string) { s.hideInLog(secrets) }

// takeLogLocked hands over the session's log to close, once, when the session ends. The caller holds the mutex and
// closes the log after letting it go: closing writes the rest of the screen to the disk.
func (s *Session) takeLogLocked() (OutputLog, func()) {
	log, stop := s.log, s.stopLogWatch
	s.log, s.stopLogWatch = nil, nil
	return log, stop
}

// closeLog says the session ended and closes its log.
func closeLog(log OutputLog, stop func()) {
	if stop != nil {
		stop()
	}
	if log != nil {
		log.Note(logEndedNote)
		log.Close()
	}
}
