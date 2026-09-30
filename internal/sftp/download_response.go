package sftp

import (
	"context"
	"io"
)

// OwnedDownloadRequest names what one response of a download job sends: the
// job's prepared bytes from Offset, pinned to Revision.
type OwnedDownloadRequest struct {
	JobID    string
	Prepared *PreparedDownload
	// Revision is what the browser holds for the prepared content (the
	// response's ETag). Its checkpoints and resumed requests name it.
	Revision string
	// Offset is where the response starts: the browser's durable checkpoint.
	Offset int64
}

// PinnedDownload is one download response that BeginOwnedDownload pinned to
// its revision. Send writes its body.
type PinnedDownload struct {
	manager *TransferManager
	request OwnedDownloadRequest
}

// BeginOwnedDownload pins the job to the request's revision and offset (see
// BeginDownload). It is separate from Send so that the caller can refuse the
// request before it writes the response headers, and write them before the
// body.
func (m *TransferManager) BeginOwnedDownload(request OwnedDownloadRequest) (*PinnedDownload, error) {
	if _, err := m.BeginDownload(request.JobID, request.Prepared.Size, request.Revision, request.Offset); err != nil {
		return nil, err
	}
	return &PinnedDownload{manager: m, request: request}, nil
}

// Send writes the prepared bytes from the offset to destination. Each write
// raises the bytes the job records as sent, and the end of the body records
// them once more, which VerifyOwnedDownload later requires. written counts
// the bytes destination took: once it is not zero the response has begun, and
// an error can only end it.
func (download *PinnedDownload) Send(ctx context.Context, destination io.Writer) (written int64, err error) {
	request := download.request
	recordSent := func(sent int64) error {
		_, err := download.manager.RecordDownloadSent(request.JobID, request.Offset+sent, request.Prepared.Size, request.Revision)
		return err
	}
	output := &sentRecordingWriter{destination: destination, recordSent: recordSent}
	if _, err := request.Prepared.WriteFrom(ctx, request.Offset, output); err != nil {
		return output.written, err
	}
	return output.written, recordSent(output.written)
}

// VerifyOwnedDownload confirms, without sending anything, that an earlier
// response of this engine sent the whole prepared revision. It first pins the
// job at the browser's durable checkpoint, so a revision that changed since
// is refused as a resumed request would be.
func (m *TransferManager) VerifyOwnedDownload(id string, prepared *PreparedDownload, revision string) error {
	checkpoint, err := m.downloadCheckpoint(id)
	if err != nil {
		return err
	}
	if _, err := m.BeginDownload(id, prepared.Size, revision, checkpoint); err != nil {
		return err
	}
	_, err = m.VerifyDownloadComplete(id, prepared.Size, revision)
	return err
}

// downloadCheckpoint is the offset the browser last reported as durable.
func (m *TransferManager) downloadCheckpoint(id string) (int64, error) {
	if !transferIDPattern.MatchString(id) {
		return 0, ErrInvalidTransfer
	}
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	record := m.jobs[id]
	if record == nil {
		return 0, ErrTransferNotFound
	}
	return record.job.TransferredBytes, nil
}

// sentRecordingWriter passes writes to destination and reports the running
// total of bytes it took after each successful write.
type sentRecordingWriter struct {
	destination io.Writer
	recordSent  func(written int64) error
	written     int64
}

func (writer *sentRecordingWriter) Write(contents []byte) (int, error) {
	written, err := writer.destination.Write(contents)
	writer.written += int64(written)
	if err == nil {
		err = writer.recordSent(writer.written)
	}
	return written, err
}
