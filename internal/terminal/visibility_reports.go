package terminal

import (
	"strings"
)

// 可視の報告（モード 2033、https://rockorager.dev/misc/visibility-reports/）は、ターミナルを誰かが見ているかも
// しれないかをプログラムに伝える。誰も見ていないあいだ、プログラムは描くのをやめられる。答えるのは
// ブラウザではなくエンジンである。セッションは、いくつものブラウザに同時に出ることも、どこにも出ないことも
// ある。つながっているブラウザのどれかがターミナルを出していると言うあいだ、見えている。何もつながって
// いなければ、見えていない。

// visibilityRequest は、プログラムが可視の報告について求めたことである。
type visibilityRequest uint8

const (
	// enableVisibilityReports は DECSET 2033 である。今すぐ報告し、変わるたびに報告する。
	enableVisibilityReports visibilityRequest = iota + 1
	// disableVisibilityReports は DECRST 2033 と、ターミナルのリセット（RIS）である。
	disableVisibilityReports
	// queryVisibility は CSI ? 998 n である。モードにかかわらず、1度だけ報告する。
	queryVisibility
	// queryVisibilityMode は DECRQM 2033 である。報告がオンかを尋ねる。
	queryVisibilityMode
)

// visibilityRequestObserver は、出力から可視の報告の求めを拾う。oscObserver と同じく、バイトは取り除かない。
// ブラウザの xterm.js はこれらのシーケンスを無視する（DECRQM 2033 は無視するよう指定してあるので、その
// モードを知らないと答えることも無い）。
type visibilityRequestObserver struct {
	controlSequenceScanner
	request func(visibilityRequest)
}

func newVisibilityRequestObserver(request func(visibilityRequest)) *visibilityRequestObserver {
	observer := &visibilityRequestObserver{request: request}
	observer.sequence = observer.complete
	observer.reset = func() { request(disableVisibilityReports) }
	return observer
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

// visibilityReport は CSI ? 999 ; Ps n である。ターミナルが見えているかもしれなければ 1、見えていなければ 2。
func visibilityReport(visible bool) []byte {
	if visible {
		return []byte("\x1b[?999;1n")
	}
	return []byte("\x1b[?999;2n")
}

// visibilityModeReport は、モード 2033 への DECRQM の答えである。オンなら 1、オフなら 2。
func visibilityModeReport(enabled bool) []byte {
	if enabled {
		return []byte("\x1b[?2033;1$y")
	}
	return []byte("\x1b[?2033;2$y")
}

// maxQueuedReplies は、入力を読まないプログラムへ書くのを待っている返事の数の上限である。超えた分は、出力を
// 止めずに捨てる（返事は、出力を読むのとは別に書く）。
const maxQueuedReplies = 16

// acceptVisibilityRequest は、プログラムの求めに、求めた世代で答える。
func (s *Session) acceptVisibilityRequest(generation uint64, request visibilityRequest) {
	s.mutex.Lock()
	if generation != s.generation || s.exited != nil {
		s.mutex.Unlock()
		return
	}
	var reply []byte
	switch request {
	case enableVisibilityReports:
		// 仕様は、報告がすでにオンでも、DECSET のたびに報告するよう求めている。
		s.shell.visibilityReports = true
		reply = visibilityReport(s.visible)
	case disableVisibilityReports:
		s.shell.visibilityReports = false
	case queryVisibility:
		reply = visibilityReport(s.visible)
	case queryVisibilityMode:
		reply = visibilityModeReport(s.shell.visibilityReports)
	}
	s.mutex.Unlock()
	s.queueReply(reply)
}

// SetStreamVisible は、stream の先のブラウザがターミナルを出しているかを記録する。何も言わない stream は、
// 出しているものとして扱う。仕様は、分からないときはターミナルを見えているものとして数える。
func (s *Session) SetStreamVisible(stream *Stream, visible bool) {
	s.mutex.Lock()
	stream.visible = visible
	reply := s.visibilityChangedLocked()
	s.mutex.Unlock()
	s.queueReply(reply)
}

// visibilityChangedLocked は、誰かがセッションを見ているかもしれないかを決め直す。それが変わり、報告がオンなら、
// 送る報告を返す。
func (s *Session) visibilityChangedLocked() []byte {
	visible := false
	for stream := range s.streams {
		visible = visible || stream.visible
	}
	if visible == s.visible {
		return nil
	}
	s.visible = visible
	if !s.shell.visibilityReports {
		return nil
	}
	return visibilityReport(visible)
}

// queueReply は、返事を、返事を書く側に渡す。問いを見つけるのは出力を読む側で、そちらは、入力を読まない
// プログラムを待ってはならない。
func (s *Session) queueReply(reply []byte) {
	if len(reply) == 0 {
		return
	}
	select {
	case s.replies <- reply:
	default:
	}
}

// writeReplies は、溜まった返事を、順にプログラムへ書く。これを始めた pump が終わる（done）まで続ける。
// 手動の再接続は、新しい done を持つ新しい pump を始め、それと一緒に新しい書き手も始める。
func (s *Session) writeReplies(done <-chan struct{}) {
	for {
		select {
		case reply := <-s.replies:
			_, _ = s.Write(reply)
		case <-done:
			return
		}
	}
}
