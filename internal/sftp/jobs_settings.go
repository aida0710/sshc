package sftp

import "time"

const (
	DefaultTransferConcurrency = 2
	MaxTransferConcurrency     = 8
	DefaultLargeFileThreshold  = int64(100 << 20)
	MinLargeFileThreshold      = int64(16 << 20)
	MaxLargeFileThreshold      = int64(1 << 30)
	// One connection by default: a single pipelined SFTP stream already fills
	// most links, and a second connection is where hosts with one-time codes,
	// per-user session caps or slow authentication start to fail. Parallel
	// ranges are opted into per engine or per CLI run.
	DefaultLargeFileParallelism = 1
	MaxLargeFileParallelism     = 128
	DefaultLargeFileChunkBytes  = int64(32 << 20)
	MinLargeFileChunkBytes      = int64(8 << 20)
	MaxLargeFileChunkBytes      = int64(4 << 30)

	// 完了項目の自動消去は、押し忘れても消える程度に長く、履歴として頼れる
	// ほどには短い範囲に収める。0 は自動消去なしである。
	MinClearCompletedAfter = 10 * time.Second
	MaxClearCompletedAfter = 24 * time.Hour
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
	m.signalSlotLocked()
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

// largeFileSplit is how a job divides a large file into ranges.
type largeFileSplit struct {
	threshold   int64
	parallelism int
	chunkBytes  int64
}

// divides says whether a file of size is transferred as ranges over
// parallel connections.
func (split largeFileSplit) divides(size int64) bool {
	return size >= split.threshold && split.parallelism > 1 && split.chunkBytes > 0
}

// largeFileSplitLocked takes the job's own values and the engine settings for
// the ones the job left at zero. StartOwned records the result in an upload
// job, so the engine settings count only until the upload first starts. A
// download reads them once, when it prepares its spool.
func (m *TransferManager) largeFileSplitLocked(job TransferJob) largeFileSplit {
	split := largeFileSplit{
		threshold: m.largeFileThreshold, parallelism: m.largeFileParallelism, chunkBytes: m.largeFileChunkBytes,
	}
	if job.LargeFileThresholdBytes != 0 {
		split.threshold = job.LargeFileThresholdBytes
	}
	if job.LargeFileParallelism != 0 {
		split.parallelism = job.LargeFileParallelism
	}
	if job.LargeFileChunkBytes != 0 {
		split.chunkBytes = job.LargeFileChunkBytes
	}
	return split
}

// uploadSplit trims the parallel connections of a split upload to what the
// host allows. Only StartOwned applies it; the ranges and the completion that
// follow use the choice StartOwned recorded, so a change of the limit takes
// effect at the next start instead of refusing ranges already in flight.
func (m *TransferManager) uploadSplit(alias string, split largeFileSplit) largeFileSplit {
	split.parallelism = m.Service.boundedParallelism(alias, split.parallelism)
	return split
}

func (m *TransferManager) LargeFileChunkBytes() int64 {
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	return m.largeFileChunkBytes
}
