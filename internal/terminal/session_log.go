package terminal

import (
	"fmt"
	"time"
)

// セッションのログは、ターミナルに出たものを文字でファイルへ書き続ける（internal/sessionlog）。書くのは、
// 接続が使える状態になったあとの出力だけである。SSH のセッションでは、シェル自身の出力だけを書く
// （ShellOutput）。認証の問いと、そこへ打った答えは書かない。

// OutputLog は、セッションのターミナルに出たものを残す（internal/sessionlog.Log）。どのメソッドもすぐ戻り、
// 書き込みはログが自分で行う。
type OutputLog interface {
	Write(output []byte)
	Resize(size Size)
	// Note は、セッションに何かが起きた時刻を伝える行を書く。
	Note(text string)
	// Hide は、これからログが書くものに、秘密の値を出さないようにする。
	Hide(secrets []string)
	// Close は、残りを書いてログを閉じる。
	Close()
}

// LogTarget は、ログを開く対象のセッションである（Registry.OpenLog）。
type LogTarget struct {
	ID   string
	Kind Kind
	// Alias は接続名である。接続名なしで開いたセッションでは行き先が入る。ローカルのシェルでは空。
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

// logOpenFailedNotice は、ファイルを作れなかったので、このセッションのログを残さないことを、ターミナルで伝える。
const logOpenFailedNotice = "\r\n[sshc] セッションのログを開けませんでした（%v）。このセッションのログは保存しません。\r\n"

// openLog は、設定がログを残すと決めているセッションについて、セッションを開くときにログを開く。
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

// startLog は、セッションの世代 generation として使えるようになった process の出力を、セッションのログで
// 追い始める。process がシェル自身の出力を渡せる（SSH のセッション）なら、ログに入るのはそれだけで、使える
// ようになる前にシェルが書いたものから始まる。渡せない（ローカルのシェル。除くべき認証が無い）なら、出力した
// ものすべてが入る。
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

// logPumpOutput は、世代 generation の process の出力をログに書く。ログがその process の出力そのものを
// 追っているあいだだけ書く（startLog）。
func (s *Session) logPumpOutput(generation uint64, output []byte) {
	s.mutex.Lock()
	log := s.log
	following := s.logPumpGeneration == generation
	s.mutex.Unlock()
	if log != nil && following {
		log.Write(output)
	}
}

// endLogOfProcess は、終わった process を追うのをやめる。つながっていた SSH の接続が切れたときは、そのことを
// ログに書く。
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

// hideInLog は、sshc がシェルに打ち込む秘密の値を、セッションのログに出さないようにする。シェルは打ち込まれた
// ものを映すことがある。
func (s *Session) hideInLog(secrets []string) {
	s.mutex.Lock()
	log := s.log
	s.mutex.Unlock()
	if log != nil && len(secrets) > 0 {
		log.Hide(secrets)
	}
}

// HideInLog は、秘密の値をセッションのログに出さないようにする。ターミナルに打ち込むコマンド（秘密の変数を
// 持つクイックコマンド）は、シェルが映す。
func (s *Session) HideInLog(secrets []string) { s.hideInLog(secrets) }

// takeLogLocked は、セッションが終わるときに、閉じるログを1度だけ渡す。呼び出し側が mutex を持ち、mutex を
// 放してからログを閉じる。閉じるときに、画面の残りをディスクに書くためである。
func (s *Session) takeLogLocked() (OutputLog, func()) {
	log, stop := s.log, s.stopLogWatch
	s.log, s.stopLogWatch = nil, nil
	return log, stop
}

// closeLog は、セッションが終わったことをログに書き、ログを閉じる。
func closeLog(log OutputLog, stop func()) {
	if stop != nil {
		stop()
	}
	if log != nil {
		log.Note(logEndedNote)
		log.Close()
	}
}
