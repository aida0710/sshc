package sftp

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net"
	"syscall"
	"time"

	pkgsftp "github.com/pkg/sftp"
)

const (
	// A bounded budget prevents unattended jobs from retrying forever.
	MaxReconnectAttempts = 10
	// Short outages get an early retry; longer outages never poll faster than this ceiling.
	initialReconnectDelay = time.Second
	maxReconnectDelay     = 30 * time.Second
)

var ErrAmbiguousTransfer = errors.New("transfer publication outcome needs reconciliation")

func publicationFailure(err error) error {
	if !ConnectionLost(err) && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return errors.Join(ErrAmbiguousTransfer, err)
}

// ConnectionLost only admits known transport failures. Authentication, host
// keys, permissions, revision conflicts and application refusals are not
// inferred from error text and are never retried.
func ConnectionLost(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, fs.ErrPermission) || errors.Is(err, fs.ErrNotExist) ||
		errors.Is(err, ErrConflict) || errors.Is(err, ErrAmbiguousTransfer) {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, pkgsftp.ErrSSHFxConnectionLost) || errors.Is(err, pkgsftp.ErrSSHFxNoConnection) {
		return true
	}
	var operation *net.OpError
	if !errors.As(err, &operation) {
		return false
	}
	return operation.Timeout() || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNABORTED) ||
		errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ENETUNREACH) || errors.Is(err, syscall.EHOSTUNREACH)
}

func validReconnectAttempts(value int) bool { return value >= 0 && value <= MaxReconnectAttempts }

func reconnectDelay(attempt int) time.Duration {
	return min(maxReconnectDelay, initialReconnectDelay*time.Duration(1<<min(max(attempt-1, 0), MaxReconnectAttempts)))
}

func (m *TransferManager) SpeedLimitBytesPerSecond() int64 {
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	return m.speedLimitBytesPerSecond
}

func (m *TransferManager) AutoReconnect() bool {
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	return m.autoReconnect
}

func (m *TransferManager) MaxReconnectAttempts() int {
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	return m.maxReconnectAttempts
}

func recoverableRemoteFile(job TransferJob) bool {
	return job.Direction == TransferRemote && job.Kind == TransferFile &&
		(job.Operation == RemoteCopy || job.Operation == RemoteGet || job.Operation == RemotePut)
}

// waitToReconnect is owned by the worker, so pause/cancel/shutdown interrupt
// both the backoff and the connection attempt. Live settings wake the wait.
func (m *TransferManager) waitToReconnect(run *remoteRun, id string) bool {
	job, err := m.reportRemoteRun(run, id, UpdateTransferJob{Action: TransferReconnectAction})
	if err != nil {
		return false
	}
	for {
		m.jobsMutex.Lock()
		record := m.jobs[id]
		if record == nil || record.job.Status != TransferReconnecting || m.processingStopped || !m.autoReconnect || record.job.ReconnectAttempt > m.maxReconnectAttempts {
			m.jobsMutex.Unlock()
			return false
		}
		changed := m.slotReleased
		delay := time.Until(job.ReconnectAt)
		m.jobsMutex.Unlock()
		if delay <= 0 {
			_, err := m.reportRemoteRun(run, id, UpdateTransferJob{Action: TransferStartAction})
			return err == nil
		}
		timer := time.NewTimer(delay)
		select {
		case <-run.ctx.Done():
			timer.Stop()
			return false
		case <-changed:
			timer.Stop()
		case <-timer.C:
		}
	}
}

func closeTransferRemote(remote Remote, operationErr error) {
	if operationErr != nil {
		discardRemote(remote)
		return
	}
	_ = remote.Close()
}
