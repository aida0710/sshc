package terminal

import "time"

// 一覧とペインに出す名前（利用者が付けた名前、プログラムが OSC で付けた名前、
// 接続先の名前）と、OSC で届いた通知を扱う。

// TitleSource は、表示する名前がどこから来たかを表す。画面が「自動の名前に戻す」を、利用者が名前を固定した
// ときだけ出せるようにする。
type TitleSource string

const (
	TitleUser       TitleSource = "user"
	TitleTerminal   TitleSource = "terminal"
	TitleConnection TitleSource = "connection"
	TitleFallback   TitleSource = "fallback"
)

// Presentation は、名前の状態のうち、表示にだけ使う部分である。
type Presentation struct {
	DisplayTitle string
	TitleSource  TitleSource
	TitlePinned  bool
}

// Rename は一覧に出す名前を変える。
func (s *Session) Rename(title string) error {
	cleaned, err := CleanTitle(title)
	if err != nil {
		return err
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.title = cleaned
	s.titleSource = TitleUser
	s.titlePinned = true
	return nil
}

// UnpinTitle は、表示する名前を、ターミナルが決めた名前か、接続名による代わりの名前に戻す。走っている
// process には触れない。
func (s *Session) UnpinTitle() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.titlePinned = false
	s.recomputeTitleLocked()
}

// acceptTitle は、OSC 0/1/2 の名前を記録する。空の名前は、プログラムが名前を消したことを表すので、ペインは
// 接続名に戻る。
//
// どのプログラムが名前を送ったかも記録する。走っているプログラムが自分で送った名前は、そのまま出す。
// シェルがプロンプトで送った名前には、走っているコマンドの名前を前に付ける（running_command_title.go）。
func (s *Session) acceptTitle(generation uint64, title string) {
	program, running := s.foregroundProgram()
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if generation != s.generation || s.exited != nil {
		return
	}
	s.shell.title = title
	s.shell.titleGroup = 0
	if running {
		s.shell.titleGroup = program.Group
	}
	if directory, ok := workingDirectoryFromTitle(title); ok {
		s.noteWorkingDirectoryLocked(directory)
	}
	s.recomputeTitleLocked()
}

// acceptNotification は、OSC 9/99/777 の通知を記録する。クライアントは、問い合わせのたびに
// NotificationVersion を比べるので、文が同じでも、求めの1つ1つを数える。
func (s *Session) acceptNotification(generation uint64, title, body string, occurredAt time.Time) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if generation != s.generation || s.exited != nil {
		return
	}
	s.notificationVersion++
	s.lastNotification = &Notification{Title: title, Body: body, OccurredAt: occurredAt}
}

func (s *Session) recomputeTitleLocked() {
	if s.titlePinned {
		s.titleSource = TitleUser
		return
	}
	if s.shell.title != "" {
		s.title = s.shell.title
		s.titleSource = TitleTerminal
		return
	}
	s.title = s.fallbackTitle
	if s.alias != "" {
		s.titleSource = TitleConnection
	} else {
		s.titleSource = TitleFallback
	}
}
