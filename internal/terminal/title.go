package terminal

import "time"

// 一覧とペインに出す名前（利用者が付けた名前、プログラムが OSC で付けた名前、
// 接続先の名前）と、OSC で届いた通知を扱う。

// TitleSource says where the display title came from so the UI can offer
// "return to the automatic name" only when a user pinned one.
type TitleSource string

const (
	TitleUser       TitleSource = "user"
	TitleTerminal   TitleSource = "terminal"
	TitleConnection TitleSource = "connection"
	TitleFallback   TitleSource = "fallback"
)

// Presentation is the display-only view of the title state.
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

// UnpinTitle returns the display title to the title the terminal set or the
// connection fallback without touching the running process.
func (s *Session) UnpinTitle() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.titlePinned = false
	s.recomputeTitleLocked()
}

// acceptTitle records an OSC 0/1/2 title. An empty title means the program
// cleared it, so the pane falls back to its connection name.
func (s *Session) acceptTitle(generation uint64, title string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if generation != s.generation || s.exited != nil {
		return
	}
	s.terminalTitle = title
	s.recomputeTitleLocked()
}

// acceptNotification records an OSC 9/99/777 notification. Clients compare
// NotificationVersion between polls, so every request counts even when the
// text repeats.
func (s *Session) acceptNotification(generation uint64, title, body string, occurredAt time.Time) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if generation != s.generation || s.exited != nil {
		return
	}
	s.notificationVersion++
	s.lastNotification = &Notification{Title: title, Body: body, OccurredAt: occurredAt}
}

func (s *Session) resetTerminalTitleLocked() {
	if s.terminalTitle == "" {
		return
	}
	s.terminalTitle = ""
	s.recomputeTitleLocked()
}

func (s *Session) recomputeTitleLocked() {
	if s.titlePinned {
		s.titleSource = TitleUser
		return
	}
	if s.terminalTitle != "" {
		s.title = s.terminalTitle
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
