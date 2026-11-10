package terminal

import (
	"strconv"
	"strings"
)

type ProgramProgressState string

const (
	ProgramProgressNormal        ProgramProgressState = "normal"
	ProgramProgressError         ProgramProgressState = "error"
	ProgramProgressIndeterminate ProgramProgressState = "indeterminate"
	ProgramProgressPaused        ProgramProgressState = "paused"
	// OSC 9;4 は3桁までを受け取り、百分率の範囲に丸める。
	maxProgramProgressDigits  = 3
	maxProgramProgressPercent = 100
)

// ProgramProgress は、今の OSC 9;4 の報告である。nil のポインターは、消されたことを表す。どこまで進んだか
// 分からない報告では、Percent に意味は無い。
type ProgramProgress struct {
	State   ProgramProgressState
	Percent int
}

// nextProgramProgress は、ブラウザの nextTerminalProgress と同じ決まりに従う。有効な値の無い error や pause で
// 百分率を保つことも同じである。
func nextProgramProgress(previous *ProgramProgress, payload string) *ProgramProgress {
	parts := strings.SplitN(payload, ";", 4)
	if len(parts) < 2 || parts[0] != "4" {
		return previous
	}
	var state ProgramProgressState
	switch parts[1] {
	case "0":
		return nil
	case "1":
		state = ProgramProgressNormal
	case "2":
		state = ProgramProgressError
	case "3":
		return &ProgramProgress{State: ProgramProgressIndeterminate}
	case "4":
		state = ProgramProgressPaused
	default:
		return previous
	}
	percent := 0
	if len(parts) > 2 && len(parts[2]) <= maxProgramProgressDigits && isDigits(parts[2]) {
		percent, _ = strconv.Atoi(parts[2])
		percent = min(percent, maxProgramProgressPercent)
	} else {
		if state == ProgramProgressNormal {
			return previous
		}
		if previous != nil && previous.State != ProgramProgressIndeterminate {
			percent = previous.Percent
		}
	}
	return &ProgramProgress{State: state, Percent: percent}
}

func (s *Session) acceptProgramProgress(generation uint64, progress *ProgramProgress) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if generation != s.generation || s.exited != nil {
		return
	}
	s.shell.programProgress = progress
}
