package sftp

import (
	"time"
)

func (m *TransferManager) ConfigureJobs(maxConcurrent int, now func() time.Time) {
	if maxConcurrent <= 0 {
		maxConcurrent = DefaultTransferConcurrency
	}
	if now == nil {
		now = time.Now
	}
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.maxConcurrent = maxConcurrent
	m.largeFileThreshold = DefaultLargeFileThreshold
	m.largeFileParallelism = DefaultLargeFileParallelism
	m.largeFileChunkBytes = DefaultLargeFileChunkBytes
	m.now = now
	if m.jobs == nil {
		m.jobs = make(map[string]*transferJobRecord)
	}
	if m.dataPlane == nil {
		m.dataPlane = make(map[string]int)
	}
}

// KeepJobActive protects an in-flight server-owned data operation from the
// stale-running sweep. It covers long remote hashing/spooling periods where no
// response bytes are available yet to report as progress.
func (m *TransferManager) KeepJobActive(id string) (func(), error) {
	if !transferIDPattern.MatchString(id) {
		return nil, ErrInvalidTransfer
	}
	m.jobsMutex.Lock()
	m.initializeJobsLocked()
	if m.jobs[id] == nil {
		m.jobsMutex.Unlock()
		return nil, ErrTransferNotFound
	}
	m.dataPlane[id]++
	m.jobsMutex.Unlock()
	return func() {
		m.jobsMutex.Lock()
		if m.dataPlane[id] <= 1 {
			delete(m.dataPlane, id)
		} else {
			m.dataPlane[id]--
		}
		m.jobsMutex.Unlock()
	}, nil
}

func (m *TransferManager) MaxConcurrent() int {
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	return m.maxConcurrent
}

// ClearCompletedAfter は、完了と取消の記録を自動で消すまでの時間である。
// 0 は自動消去なしを意味する。
func (m *TransferManager) ClearCompletedAfter() time.Duration {
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	return m.clearCompletedAfter
}

// ProcessingStopped は、待機中の job を新しく開始しない状態かどうかである。
func (m *TransferManager) ProcessingStopped() bool {
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	return m.processingStopped
}

func (m *TransferManager) LargeFileThreshold() int64 {
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	return m.largeFileThreshold
}

func (m *TransferManager) LargeFileParallelism() int {
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	return m.largeFileParallelism
}

func (m *TransferManager) LargeFileChunkBytes() int64 {
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	return m.largeFileChunkBytes
}

// SetTransferSettings は、同時転送数、完了項目の自動消去時間、キュー処理の
// 停止を差し替える。
//
// 転送は engine の資源であって browser のものではないため、値は engine 側に
// 一つだけ置く。永続化は呼び出し側の責務である。
func (m *TransferManager) SetTransferSettings(
	maxConcurrent int, clearCompletedAfter time.Duration, processingStopped bool,
	largeFileThreshold int64, largeFileParallelism int, largeFileChunkBytes int64,
) error {
	if maxConcurrent < 1 || maxConcurrent > MaxTransferConcurrency {
		return ErrInvalidTransfer
	}
	if clearCompletedAfter != 0 && (clearCompletedAfter < MinClearCompletedAfter || clearCompletedAfter > MaxClearCompletedAfter) {
		return ErrInvalidTransfer
	}
	if largeFileThreshold < MinLargeFileThreshold || largeFileThreshold > MaxLargeFileThreshold ||
		largeFileParallelism < 1 || largeFileParallelism > MaxLargeFileParallelism ||
		largeFileChunkBytes < MinLargeFileChunkBytes || largeFileChunkBytes > MaxLargeFileChunkBytes {
		return ErrInvalidTransfer
	}
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	m.maxConcurrent = maxConcurrent
	m.clearCompletedAfter = clearCompletedAfter
	m.processingStopped = processingStopped
	m.largeFileThreshold = largeFileThreshold
	m.largeFileParallelism = largeFileParallelism
	m.largeFileChunkBytes = largeFileChunkBytes
	return nil
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
