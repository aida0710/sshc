package terminal

import "strings"

// programStatusRecords は、1つのセッションの OSC 7501 の記録を、id ごとに持つ。"" は根である。記録は画面では
// なくセッションのものなので、代替画面は記録に触れない。done と error の記録は、報告が置き換えるか消すまで
// 残る。出したと決めるのはブラウザである（Version を参照）。
type programStatusRecords struct {
	byID map[string]ProgramStatus
	// updates は、報告に番号を振る。消したあとも増え続けるので、記録の Version がセッションの中で繰り返すことは
	// 無い。
	updates uint64
}

// apply は、報告1つを受け取る。報告は記録を丸ごと置き換えるので、報告に無いキーは無くなる。
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

// drop は、states のどれかにある記録を消す。
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

// hasLiveReport は、プログラムが、今していることを伝えているかを返す。done と failed の記録は数えない。
// プログラムが去ったあとも残るためである。
func (r *programStatusRecords) hasLiveReport() bool {
	for id, record := range r.byID {
		if id == screenPromptRecordID {
			continue
		}
		switch record.State {
		case ProgramStatusIdle, ProgramStatusWorking, ProgramStatusBlocked:
			return true
		}
	}
	return false
}

// programStatusUrgency は、状態を、利用者をどれだけ必要としているかで並べる。まだ何かに取り組んでいる
// プログラムは、その一部が終わっていても、働いているものとして出す。
var programStatusUrgency = map[ProgramStatusState]int{
	ProgramStatusIdle:    0,
	ProgramStatusDone:    1,
	ProgramStatusWorking: 2,
	ProgramStatusError:   3,
	ProgramStatusBlocked: 4,
}

// summary は、セッションの一覧に出す記録である。いちばん急ぎのもの、同じならその中でいちばん新しいものを
// 選び、それが属するプログラムの名前を付ける。無ければ nil。
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

// appOf は、記録 id の名前か、名前を持ついちばん近い親の名前である。あいだに記録が無くても、親として数える。
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

// parentProgramStatusID は、区切り1つ上のパスである。根の記録 "" は、すべての記録の親である。id の無い clear が
// すべてを消すのと同じ扱いである。
func parentProgramStatusID(id string) string {
	index := strings.LastIndexByte(id, '/')
	if index < 0 {
		return ""
	}
	return id[:index]
}

// acceptProgramStatus は、報告か問い合わせを、送った世代から受け取る。
func (s *Session) acceptProgramStatus(generation uint64, report programStatusReport) {
	s.mutex.Lock()
	if generation != s.generation || s.exited != nil {
		s.mutex.Unlock()
		return
	}
	if !report.query {
		s.programStatus.apply(report)
		s.reportBlockedProgramLocked()
	}
	s.mutex.Unlock()
	if report.query {
		s.queueReply([]byte(programStatusAnswer))
	}
}

// endActiveProgramStatusLocked は、プログラムが走っているあいだだけ成り立つものを捨てる。シェルの新しい
// プロンプト（OSC 133;A）は、プログラムが戻ったことを意味する。古い process に替わる新しい process も
// 同じである。
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
	s.reportBlockedProgramLocked()
}

// acceptTerminalReset は、RIS に従う。RIS はすべての記録を消す。
func (s *Session) acceptTerminalReset(generation uint64) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if generation != s.generation || s.exited != nil {
		return
	}
	s.programStatus.clear("")
	s.reportBlockedProgramLocked()
}
