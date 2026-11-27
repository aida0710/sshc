package terminal

import "slices"

// MoveToWorkspace は、セッションを workspace のワークスペースへ移し、一覧の中で beforeID のセッションの
// 前に置く。beforeID が空なら一覧の末尾に置くので、そのワークスペースの最後になる。画面はワークスペース
// ごとに一覧の順でセッションを並べるので、どのブラウザ・マシンから開いても同じ順に並ぶ。
// beforeID のセッションが無いか、workspace に入っていなければ ErrNotFound を返し、何も変えない。
// ワークスペースがあるかどうかは呼び出し側が確かめる。
func (r *Registry) MoveToWorkspace(id, workspace, beforeID string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	from := r.indexOf(id)
	if from < 0 {
		return ErrNotFound
	}
	if beforeID != "" && beforeID != id {
		before := r.indexOf(beforeID)
		if before < 0 || r.sessions[before].Workspace() != workspace {
			return ErrNotFound
		}
	}
	moved := r.sessions[from]
	moved.mutex.Lock()
	moved.workspace = workspace
	moved.mutex.Unlock()
	if beforeID == id {
		return nil
	}
	r.sessions = slices.Delete(r.sessions, from, from+1)
	at := len(r.sessions)
	if beforeID != "" {
		at = r.indexOf(beforeID)
	}
	r.sessions = slices.Insert(r.sessions, at, moved)
	return nil
}

// Workspace は、このセッションが入っているワークスペースの ID を返す。
func (s *Session) Workspace() string {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.workspace
}

// indexOf は、一覧の中の id のセッションの位置を返す。無ければ -1。r.mutex を持って呼ぶ。
func (r *Registry) indexOf(id string) int {
	return slices.IndexFunc(r.sessions, func(session *Session) bool { return session.id == id })
}

// MoveWorkspaceSessions は、from のワークスペースに入っているセッションを、終了した
// ものも含めてすべて to へ移す。ワークスペースを消すときに使う。
func (r *Registry) MoveWorkspaceSessions(from, to string) {
	for _, session := range r.listedSessions() {
		session.mutex.Lock()
		if session.workspace == from {
			session.workspace = to
		}
		session.mutex.Unlock()
	}
}
