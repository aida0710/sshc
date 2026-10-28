package terminal

import (
	"strings"
)

// Visibility reports (mode 2033, https://rockorager.dev/misc/visibility-reports/)
// tell a program whether anyone may see its terminal, so it can stop drawing
// while nobody does. The engine answers, not a browser: a session can be shown
// in several browsers at once, or in none. It is visible while any attached
// browser says it shows the terminal; with nothing attached it is not.

// visibilityRequest is what a program asked of visibility reports.
type visibilityRequest uint8

const (
	// enableVisibilityReports is DECSET 2033: report now and on every change.
	enableVisibilityReports visibilityRequest = iota + 1
	// disableVisibilityReports is DECRST 2033, and a terminal reset (RIS).
	disableVisibilityReports
	// queryVisibility is CSI ? 998 n: report once, whatever the mode.
	queryVisibility
	// queryVisibilityMode is DECRQM 2033: whether reports are on.
	queryVisibilityMode
)

// maxControlSequenceBytes bounds what one control sequence may collect. The
// requests here are under 16 bytes; a longer sequence is something else.
const maxControlSequenceBytes = 32

type controlSequenceState uint8

const (
	sequenceText controlSequenceState = iota
	sequenceEscape
	sequenceCollect
	sequenceDiscard
)

// visibilityRequestObserver picks the visibility-report requests out of the
// output. Like oscObserver it never removes bytes; xterm.js in the browser
// ignores these sequences (it is told to for DECRQM 2033, so it does not
// answer that it does not know the mode).
type visibilityRequestObserver struct {
	state   controlSequenceState
	collect []byte
	request func(visibilityRequest)
}

func newVisibilityRequestObserver(request func(visibilityRequest)) *visibilityRequestObserver {
	return &visibilityRequestObserver{request: request}
}

// Observe scans one chunk. Sequences may span chunk boundaries.
func (o *visibilityRequestObserver) Observe(chunk []byte) {
	for _, value := range chunk {
		switch o.state {
		case sequenceText:
			if value == '\x1b' {
				o.state = sequenceEscape
			}
		case sequenceEscape:
			switch value {
			case '[':
				o.state = sequenceCollect
				o.collect = o.collect[:0]
			case 'c':
				o.state = sequenceText
				o.request(disableVisibilityReports)
			case '\x1b':
			default:
				o.state = sequenceText
			}
		case sequenceCollect, sequenceDiscard:
			o.next(value)
		}
	}
}

// next takes one byte of a control sequence: its parameters and intermediates
// up to the final byte.
func (o *visibilityRequestObserver) next(value byte) {
	switch {
	case value == '\x1b':
		o.state = sequenceEscape
	case value >= 0x40 && value <= 0x7e:
		if o.state == sequenceCollect {
			o.complete(string(o.collect), value)
		}
		o.state = sequenceText
	case value < 0x20 || o.state == sequenceDiscard:
		// C0 controls inside a sequence act on their own and take no part in it.
	case len(o.collect) >= maxControlSequenceBytes:
		o.state = sequenceDiscard
	default:
		o.collect = append(o.collect, value)
	}
}

func (o *visibilityRequestObserver) complete(parameters string, final byte) {
	private, ok := strings.CutPrefix(parameters, "?")
	if !ok {
		return
	}
	switch final {
	case 'h', 'l':
		for _, mode := range strings.Split(private, ";") {
			if mode != "2033" {
				continue
			}
			if final == 'h' {
				o.request(enableVisibilityReports)
			} else {
				o.request(disableVisibilityReports)
			}
		}
	case 'n':
		if private == "998" {
			o.request(queryVisibility)
		}
	case 'p':
		if private == "2033$" {
			o.request(queryVisibilityMode)
		}
	}
}

// visibilityReport is CSI ? 999 ; Ps n: 1 when the terminal may be seen, 2 when it is not.
func visibilityReport(visible bool) []byte {
	if visible {
		return []byte("\x1b[?999;1n")
	}
	return []byte("\x1b[?999;2n")
}

// visibilityModeReport is the DECRQM answer for mode 2033: 1 set, 2 reset.
func visibilityModeReport(enabled bool) []byte {
	if enabled {
		return []byte("\x1b[?2033;1$y")
	}
	return []byte("\x1b[?2033;2$y")
}

// maxQueuedReplies bounds the answers waiting to be written to a program that
// does not read its input; past it they are dropped rather than holding up the
// output (the answers are written apart from the reading of the output).
const maxQueuedReplies = 16

// acceptVisibilityRequest answers what the program asked, in the generation
// that asked it.
func (s *Session) acceptVisibilityRequest(generation uint64, request visibilityRequest) {
	s.mutex.Lock()
	if generation != s.generation || s.exited != nil {
		s.mutex.Unlock()
		return
	}
	var reply []byte
	switch request {
	case enableVisibilityReports:
		// The spec asks for a report on every DECSET, even when reports were on.
		s.visibilityReports = true
		reply = visibilityReport(s.visible)
	case disableVisibilityReports:
		s.visibilityReports = false
	case queryVisibility:
		reply = visibilityReport(s.visible)
	case queryVisibilityMode:
		reply = visibilityModeReport(s.visibilityReports)
	}
	s.mutex.Unlock()
	s.queueReply(reply)
}

// SetStreamVisible records whether the browser behind stream shows its
// terminal. A stream that never says is taken to show it: in doubt, the spec
// counts a terminal as visible.
func (s *Session) SetStreamVisible(stream *Stream, visible bool) {
	s.mutex.Lock()
	stream.visible = visible
	reply := s.visibilityChangedLocked()
	s.mutex.Unlock()
	s.queueReply(reply)
}

// visibilityChangedLocked works out again whether anyone may see the session,
// and returns the report to send when that changed and reports are on.
func (s *Session) visibilityChangedLocked() []byte {
	visible := false
	for stream := range s.streams {
		visible = visible || stream.visible
	}
	if visible == s.visible {
		return nil
	}
	s.visible = visible
	if !s.visibilityReports {
		return nil
	}
	return visibilityReport(visible)
}

// queueReply hands an answer to the writer of answers. The reading of the
// output, which finds the questions, must not wait on a program that does not
// read its input.
func (s *Session) queueReply(reply []byte) {
	if len(reply) == 0 {
		return
	}
	select {
	case s.replies <- reply:
	default:
	}
}

// writeReplies writes the queued answers to the program, in order, until the
// session ends.
func (s *Session) writeReplies() {
	for {
		select {
		case reply := <-s.replies:
			_, _ = s.Write(reply)
		case <-s.done:
			return
		}
	}
}
