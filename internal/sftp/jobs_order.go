package sftp

// TransferQueueMove は、待機中の job を待機列の中でどこへ動かすかである。
// running の job は動かさない。動かせば、いま走っているものが止まる。
type TransferQueueMove string

const (
	TransferMoveUp     TransferQueueMove = "up"
	TransferMoveDown   TransferQueueMove = "down"
	TransferMoveTop    TransferQueueMove = "top"
	TransferMoveBottom TransferQueueMove = "bottom"
)

func (m *TransferManager) removeJobOrderLocked(id string) {
	for index, candidate := range m.jobOrder {
		if candidate == id {
			m.jobOrder = append(m.jobOrder[:index], m.jobOrder[index+1:]...)
			return
		}
	}
}

func (m *TransferManager) jobOrderIndexLocked(id string) int {
	for index, candidate := range m.jobOrder {
		if candidate == id {
			return index
		}
	}
	return len(m.jobOrder)
}

func (m *TransferManager) insertJobOrderLocked(index int, id string) {
	index = min(max(index, 0), len(m.jobOrder))
	m.jobOrder = append(m.jobOrder, "")
	copy(m.jobOrder[index+1:], m.jobOrder[index:])
	m.jobOrder[index] = id
}

// MoveQueuedJob は、待機中の job だけを待機列の中で入れ替える。running や
// paused の位置は動かないので、並べ替えても走っている転送は影響を受けない。
func (m *TransferManager) MoveQueuedJob(id string, move TransferQueueMove) error {
	if !transferIDPattern.MatchString(id) {
		return ErrInvalidTransfer
	}
	switch move {
	case TransferMoveUp, TransferMoveDown, TransferMoveTop, TransferMoveBottom:
	default:
		return ErrInvalidTransfer
	}
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	record := m.jobs[id]
	if record == nil {
		return ErrTransferNotFound
	}
	if record.job.Status != TransferQueued {
		return ErrTransferState
	}
	slots := make([]int, 0, len(m.jobOrder))
	current := -1
	for index, candidate := range m.jobOrder {
		waiting := m.jobs[candidate]
		if waiting == nil || waiting.job.Status != TransferQueued {
			continue
		}
		if candidate == id {
			current = len(slots)
		}
		slots = append(slots, index)
	}
	if current < 0 || len(slots) < 2 {
		return nil
	}
	destination := current
	switch move {
	case TransferMoveUp:
		destination = current - 1
	case TransferMoveDown:
		destination = current + 1
	case TransferMoveTop:
		destination = 0
	case TransferMoveBottom:
		destination = len(slots) - 1
	}
	if destination < 0 || destination >= len(slots) || destination == current {
		return nil
	}
	originalOrder := append([]string(nil), m.jobOrder...)
	// 待機中の id だけを取り出し、順番を変えて同じ位置へ書き戻す。running の
	// job が占める index は触らないので、待機列だけが並び替わる。
	waiting := make([]string, 0, len(slots))
	for _, index := range slots {
		waiting = append(waiting, m.jobOrder[index])
	}
	moved := waiting[current]
	waiting = append(waiting[:current], waiting[current+1:]...)
	waiting = append(waiting[:destination], append([]string{moved}, waiting[destination:]...)...)
	for position, index := range slots {
		m.jobOrder[index] = waiting[position]
	}
	if err := m.persistJobsLocked(true); err != nil {
		m.jobOrder = originalOrder
		return err
	}
	return nil
}
