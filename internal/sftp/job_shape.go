package sftp

// transferJobShape holds the fields that say what a transfer job does and
// where. CreateJob accepts a job only in a shape that the queue can load back
// after a restart, so both check it with validateTransferJobShape; a rule
// changed on only one side would let a saved queue be quarantined as corrupt.
type transferJobShape struct {
	ID          string
	BatchID     string
	Direction   TransferDirection
	Kind        TransferKind
	Operation   RemoteTransferOperation
	Alias       string
	SourceAlias string
	SourcePath  string
	RemotePath  string
	Overwrite   bool
	TotalBytes  int64

	LargeFileThresholdBytes int64
	LargeFileParallelism    int
	LargeFileChunkBytes     int64
}

func (input CreateTransferJob) shape() transferJobShape {
	return transferJobShape{
		ID: input.ID, BatchID: input.BatchID, Direction: input.Direction, Kind: input.Kind,
		Operation: input.Operation, Alias: input.Alias, SourceAlias: input.SourceAlias,
		SourcePath: input.SourcePath, RemotePath: input.RemotePath, Overwrite: input.Overwrite,
		TotalBytes:              input.TotalBytes,
		LargeFileThresholdBytes: input.LargeFileThresholdBytes, LargeFileParallelism: input.LargeFileParallelism,
		LargeFileChunkBytes: input.LargeFileChunkBytes,
	}
}

func (job TransferJob) shape() transferJobShape {
	return transferJobShape{
		ID: job.ID, BatchID: job.BatchID, Direction: job.Direction, Kind: job.Kind,
		Operation: job.Operation, Alias: job.Alias, SourceAlias: job.SourceAlias,
		SourcePath: job.SourcePath, RemotePath: job.RemotePath, Overwrite: job.Overwrite,
		TotalBytes:              job.TotalBytes,
		LargeFileThresholdBytes: job.LargeFileThresholdBytes, LargeFileParallelism: job.LargeFileParallelism,
		LargeFileChunkBytes: job.LargeFileChunkBytes,
	}
}

// validateTransferJobShape checks the shape of a job and returns its target
// path cleaned. The target of a get is on the engine's file system and every
// other target is on a remote host; the source of a put is local likewise.
func validateTransferJobShape(shape transferJobShape) (string, error) {
	if !transferIDPattern.MatchString(shape.ID) || !transferIDPattern.MatchString(shape.BatchID) ||
		shape.TotalBytes < -1 ||
		(shape.Direction != TransferUpload && shape.Direction != TransferDownload && shape.Direction != TransferRemote) ||
		(shape.Kind != TransferFile && shape.Kind != TransferFolder) {
		return "", ErrInvalidTransfer
	}
	if !validLargeFileOverrides(shape) {
		return "", ErrInvalidTransfer
	}
	if err := validateAlias(shape.Alias); err != nil {
		return "", err
	}
	if shape.Direction == TransferRemote {
		if err := validateRemoteOperationShape(shape); err != nil {
			return "", err
		}
	} else if shape.SourceAlias != "" || shape.SourcePath != "" || shape.Operation != "" {
		return "", ErrInvalidTransfer
	}
	if shape.Operation == RemoteGet {
		return cleanLocalPath(shape.RemotePath)
	}
	return cleanPublicPath(shape.RemotePath, false)
}

// validLargeFileOverrides accepts a job's own large file settings only within
// the engine's ranges and only on a file upload or download, the transfers
// that split a file into ranges. Zero leaves a setting to the engine.
func validLargeFileOverrides(shape transferJobShape) bool {
	overridden := shape.LargeFileThresholdBytes != 0 || shape.LargeFileParallelism != 0 || shape.LargeFileChunkBytes != 0
	splits := (shape.Direction == TransferDownload || shape.Direction == TransferUpload) && shape.Kind == TransferFile
	if overridden && !splits {
		return false
	}
	return (shape.LargeFileThresholdBytes == 0 || validLargeFileThreshold(shape.LargeFileThresholdBytes)) &&
		(shape.LargeFileParallelism == 0 || validLargeFileParallelism(shape.LargeFileParallelism)) &&
		(shape.LargeFileChunkBytes == 0 || validLargeFileChunkBytes(shape.LargeFileChunkBytes))
}

// validateRemoteOperationShape checks the source side of a job that runs on
// the engine. A get or put moves files between one host and the engine, so its
// source host is the target host; a delete names one path on one host and has
// nothing to overwrite.
func validateRemoteOperationShape(shape transferJobShape) error {
	if err := validateAlias(shape.SourceAlias); err != nil {
		return err
	}
	switch shape.Operation {
	case RemoteCopy, RemoteMove, RemoteDelete, RemoteGet, RemotePut:
	default:
		return ErrInvalidTransfer
	}
	if (shape.Operation == RemoteGet || shape.Operation == RemotePut) && shape.SourceAlias != shape.Alias {
		return ErrInvalidTransfer
	}
	var source string
	var err error
	if shape.Operation == RemotePut {
		source, err = cleanLocalPath(shape.SourcePath)
	} else {
		source, err = cleanPublicPath(shape.SourcePath, false)
	}
	if err != nil {
		return err
	}
	if shape.Operation == RemoteDelete &&
		(shape.SourceAlias != shape.Alias || source != shape.SourcePath || shape.SourcePath != shape.RemotePath || shape.Overwrite) {
		return ErrInvalidTransfer
	}
	return nil
}
