package terminal

// MoveToWorkspace は、セッションを別のワークスペースへ移す。ワークスペースが
// あるかどうかは呼び出し側が確かめる。
func (r *Registry) MoveToWorkspace(id, workspace string) error {
	session, ok := r.Lookup(id)
	if !ok {
		return ErrNotFound
	}
	session.mutex.Lock()
	session.workspace = workspace
	session.mutex.Unlock()
	return nil
}

// MoveWorkspaceSessions は、from のワークスペースに入っているセッションを、終了した
// ものも含めてすべて to へ移す。ワークスペースを消すときに使う。
func (r *Registry) MoveWorkspaceSessions(from, to string) {
	r.mutex.Lock()
	sessions := append([]*Session(nil), r.sessions...)
	r.mutex.Unlock()
	for _, session := range sessions {
		session.mutex.Lock()
		if session.workspace == from {
			session.workspace = to
		}
		session.mutex.Unlock()
	}
}
