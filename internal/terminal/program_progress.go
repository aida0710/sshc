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
	// OSC 9;4 accepts up to three digits and clamps them to a percentage.
	maxProgramProgressDigits  = 3
	maxProgramProgressPercent = 100
)

// ProgramProgress is the current OSC 9;4 report. A nil pointer means cleared;
// Percent has no meaning for an indeterminate report.
type ProgramProgress struct {
	State   ProgramProgressState
	Percent int
}

// nextProgramProgress follows the browser's nextTerminalProgress rules,
// including keeping the percentage on an error or pause without a valid value.
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
	s.programProgress = progress
}
