package terminal

import "strings"

// programStatusRecords are one session's OSC 7501 records by id; "" is the
// root. They belong to the session, not to a screen, so the alternate screen
// leaves them alone. Done and error records stay until a report replaces or
// clears them: the browser decides when it has shown one (see Version).
type programStatusRecords struct {
	byID map[string]ProgramStatus
	// updates numbers the reports. It keeps growing across clears, so a
	// record's Version never repeats within the session.
	updates uint64
}

// apply takes one report. A report replaces its record whole, so a key it
// leaves out is gone.
func (r *programStatusRecords) apply(report programStatusReport) {
	if report.clear {
		r.clear(report.id)
		return
	}
	if r.byID == nil {
		r.byID = make(map[string]ProgramStatus)
	}
	r.updates++
	status := report.status
	status.Version = r.updates
	r.byID[report.id] = status
	if len(r.byID) > maxProgramStatusRecords {
		r.dropLeastRecentlyUpdated()
	}
}

func (r *programStatusRecords) clear(id string) {
	if id == "" {
		r.byID = nil
		return
	}
	for recordID := range r.byID {
		if recordID == id || strings.HasPrefix(recordID, id+"/") {
			delete(r.byID, recordID)
		}
	}
}

func (r *programStatusRecords) dropLeastRecentlyUpdated() {
	oldestID, oldest := "", uint64(0)
	for id, record := range r.byID {
		if oldest == 0 || record.Version < oldest {
			oldestID, oldest = id, record.Version
		}
	}
	delete(r.byID, oldestID)
}

// drop removes the records in any of states.
func (r *programStatusRecords) drop(states ...ProgramStatusState) {
	for id, record := range r.byID {
		for _, state := range states {
			if record.State == state {
				delete(r.byID, id)
				break
			}
		}
	}
}

// programStatusUrgency orders the states by how much they need the user. A
// program still working on something is shown working, even when one part of
// it is done.
var programStatusUrgency = map[ProgramStatusState]int{
	ProgramStatusIdle:    0,
	ProgramStatusDone:    1,
	ProgramStatusWorking: 2,
	ProgramStatusError:   3,
	ProgramStatusBlocked: 4,
}

// summary is the record the session list shows: the most urgent one, the
// latest of those on a tie, with the name of the program it belongs to. nil
// when there is none.
func (r *programStatusRecords) summary() *ProgramStatus {
	chosenID, found := "", false
	for id, record := range r.byID {
		if !found {
			chosenID, found = id, true
			continue
		}
		chosen := r.byID[chosenID]
		urgency, chosenUrgency := programStatusUrgency[record.State], programStatusUrgency[chosen.State]
		if urgency > chosenUrgency || urgency == chosenUrgency && record.Version > chosen.Version {
			chosenID = id
		}
	}
	if !found {
		return nil
	}
	status := r.byID[chosenID]
	if status.Progress != nil {
		percent := *status.Progress
		status.Progress = &percent
	}
	status.App = r.appOf(chosenID)
	return &status
}

// appOf is the name of the record id or of its nearest ancestor that has one.
// An ancestor counts whether or not a record of its own exists between them.
func (r *programStatusRecords) appOf(id string) string {
	for {
		if app := r.byID[id].App; app != "" {
			return app
		}
		if id == "" {
			return ""
		}
		id = parentProgramStatusID(id)
	}
}

// parentProgramStatusID is the path one segment up. The root record "" is
// the ancestor of every record, as a clear without an id removes them all.
func parentProgramStatusID(id string) string {
	index := strings.LastIndexByte(id, '/')
	if index < 0 {
		return ""
	}
	return id[:index]
}

// acceptProgramStatus takes a report or a query from the generation that
// sent it.
func (s *Session) acceptProgramStatus(generation uint64, report programStatusReport) {
	s.mutex.Lock()
	if generation != s.generation || s.exited != nil {
		s.mutex.Unlock()
		return
	}
	if !report.query {
		s.programStatus.apply(report)
	}
	s.mutex.Unlock()
	if report.query {
		s.queueReply([]byte(programStatusAnswer))
	}
}

// endActiveProgramStatusLocked drops what only holds while a program runs:
// a new shell prompt (OSC 133;A) means it has returned, as does a new process
// in place of the old one.
func (s *Session) endActiveProgramStatusLocked() {
	s.programStatus.drop(ProgramStatusWorking, ProgramStatusBlocked)
}

func (s *Session) acceptPromptStarted(generation uint64) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if generation != s.generation || s.exited != nil {
		return
	}
	s.endActiveProgramStatusLocked()
}

// acceptTerminalReset follows RIS, which removes every record.
func (s *Session) acceptTerminalReset(generation uint64) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if generation != s.generation || s.exited != nil {
		return
	}
	s.programStatus.clear("")
}
