package httpserver

import (
	"time"

	"sshc/internal/api"
	sshcSFTP "sshc/internal/sftp"
)

// SFTP の API の本文の形と、engine の値からその形への変換である。CLI が読む応答の型は
// 公開し、CLI も同じ型で読む。形を二か所に持つと、片方だけに項目が増えたときに、
// 未知の項目を許さない CLI の読み手が壊れる。どの型も wire_contract_test.go が
// api/openapi.yaml と突き合わせる。

// SFTPListing は、リモートのディレクトリの一覧である。
type SFTPListing struct {
	Path    string          `json:"path"`
	Entries []api.SFTPEntry `json:"entries"`
}

type sftpSearchResponse struct {
	Path      string          `json:"path"`
	Query     string          `json:"query"`
	Truncated bool            `json:"truncated"`
	Entries   []api.SFTPEntry `json:"entries"`
}

type sftpTextFileResponse struct {
	Entry    api.SFTPEntry `json:"entry"`
	Contents string        `json:"contents"`
	Revision string        `json:"revision"`
}

type sftpSaveTextRequest struct {
	Contents         string `json:"contents"`
	ExpectedRevision string `json:"expectedRevision"`
}

type sftpRenameRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type sftpChmodRequest struct {
	Path             string `json:"path"`
	Mode             string `json:"mode"`
	ExpectedRevision string `json:"expectedRevision"`
	Recursive        bool   `json:"recursive"`
}

// SFTPTransfer は、アップロードを確定したファイルである。
type SFTPTransfer struct {
	Path     string `json:"path"`
	Bytes    int64  `json:"bytes"`
	Revision string `json:"revision"`
}

type changedResponse struct {
	Changed bool `json:"changed"`
}

// describeSFTPEntry は engine の Entry を契約の SFTPEntry にする。一覧、検索、
// text、比較のすべてがこの 1 つを使う。
func describeSFTPEntry(entry sshcSFTP.Entry) api.SFTPEntry {
	described := api.SFTPEntry{
		Name: entry.Name, Path: entry.Path, Type: api.SFTPEntryType(entry.Type), Size: entry.Size,
		Mode: entry.Mode.String(), ModifiedAt: entry.ModifiedAt.UTC(), Revision: entry.Revision,
	}
	if entry.LinkTarget != "" {
		described.LinkTarget = &entry.LinkTarget
	}
	if entry.TargetType != "" {
		targetType := api.SFTPEntryTargetType(entry.TargetType)
		described.TargetType = &targetType
	}
	return described
}

// SFTPTransferJob は、転送キューの 1 つの job である。
type SFTPTransferJob struct {
	ID                string                           `json:"id"`
	BatchID           string                           `json:"batchId"`
	BatchName         string                           `json:"batchName"`
	BatchKind         sshcSFTP.TransferKind            `json:"batchKind"`
	Alias             string                           `json:"alias"`
	SourceAlias       string                           `json:"sourceAlias"`
	SourcePath        string                           `json:"sourcePath"`
	Operation         sshcSFTP.RemoteTransferOperation `json:"operation"`
	Direction         sshcSFTP.TransferDirection       `json:"direction"`
	Kind              sshcSFTP.TransferKind            `json:"kind"`
	Name              string                           `json:"name"`
	RemotePath        string                           `json:"remotePath"`
	TotalBytes        int64                            `json:"totalBytes"`
	TransferredBytes  int64                            `json:"transferredBytes"`
	BytesPerSecond    float64                          `json:"bytesPerSecond"`
	RemainingSeconds  int64                            `json:"remainingSeconds"`
	Status            sshcSFTP.TransferJobStatus       `json:"status"`
	AllowedActions    []sshcSFTP.TransferControlAction `json:"allowedActions"`
	Attempt           int                              `json:"attempt"`
	Problem           string                           `json:"problem"`
	LastModified      int64                            `json:"lastModified"`
	ExpectedRevision  string                           `json:"expectedRevision"`
	SourceFingerprint string                           `json:"sourceFingerprint"`
	Overwrite         bool                             `json:"overwrite"`
	DownloadRevision  string                           `json:"downloadRevision"`
	DownloadParts     []SFTPDownloadPartProgress       `json:"downloadParts"`
	CreatedAt         string                           `json:"createdAt"`
	UpdatedAt         string                           `json:"updatedAt"`
}

// SFTPDownloadPartProgress は、大きいファイルのダウンロードを用意する接続 1 本の進み具合である。
type SFTPDownloadPartProgress struct {
	Index            int   `json:"index"`
	TransferredBytes int64 `json:"transferredBytes"`
	TotalBytes       int64 `json:"totalBytes"`
}

func describeTransferJob(job sshcSFTP.TransferJob) SFTPTransferJob {
	parts := make([]SFTPDownloadPartProgress, 0, len(job.DownloadParts))
	for _, part := range job.DownloadParts {
		parts = append(parts, SFTPDownloadPartProgress{
			Index: part.Index, TransferredBytes: part.TransferredBytes, TotalBytes: part.TotalBytes,
		})
	}
	return SFTPTransferJob{
		ID: job.ID, BatchID: job.BatchID, BatchName: job.BatchName, BatchKind: job.BatchKind,
		Alias: job.Alias, SourceAlias: job.SourceAlias, SourcePath: job.SourcePath,
		Operation: job.Operation, Direction: job.Direction,
		Kind: job.Kind, Name: job.Name, RemotePath: job.RemotePath, TotalBytes: job.TotalBytes,
		TransferredBytes: job.TransferredBytes, BytesPerSecond: job.BytesPerSecond,
		RemainingSeconds: job.RemainingSeconds, Status: job.Status,
		AllowedActions: sshcSFTP.AllowedTransferActions(job), Attempt: job.Attempt,
		Problem: job.Problem, LastModified: job.LastModified,
		ExpectedRevision: job.ExpectedRevision, SourceFingerprint: job.SourceFingerprint,
		Overwrite: job.Overwrite, DownloadRevision: job.DownloadRevision,
		DownloadParts: parts,
		CreatedAt:     job.CreatedAt.Format(time.RFC3339Nano), UpdatedAt: job.UpdatedAt.Format(time.RFC3339Nano),
	}
}

// SFTPTransferJobList は、転送キューの設定と job の一覧である。
type SFTPTransferJobList struct {
	MaxConcurrent           int   `json:"maxConcurrent"`
	LargeFileThresholdBytes int64 `json:"largeFileThresholdBytes"`
	LargeFileParallelism    int   `json:"largeFileParallelism"`
	LargeFileChunkBytes     int64 `json:"largeFileChunkBytes"`
	// 0 は自動消去なしである。
	ClearCompletedAfterSeconds int `json:"clearCompletedAfterSeconds"`
	// 停止中は待機の job を新しく開始しない。
	ProcessingStopped bool              `json:"processingStopped"`
	Jobs              []SFTPTransferJob `json:"jobs"`
}

type sftpTransferSettingsRequest struct {
	MaxConcurrent              int   `json:"maxConcurrent"`
	ClearCompletedAfterSeconds int   `json:"clearCompletedAfterSeconds"`
	ProcessingStopped          bool  `json:"processingStopped"`
	LargeFileThresholdBytes    int64 `json:"largeFileThresholdBytes"`
	LargeFileParallelism       int   `json:"largeFileParallelism"`
	LargeFileChunkBytes        int64 `json:"largeFileChunkBytes"`
}

type sftpTransferQueueMoveRequest struct {
	Move sshcSFTP.TransferQueueMove `json:"move"`
}

type sftpCreateTransferJobRequest struct {
	ID                      string                           `json:"id"`
	BatchID                 string                           `json:"batchId"`
	BatchName               string                           `json:"batchName"`
	BatchKind               sshcSFTP.TransferKind            `json:"batchKind"`
	Alias                   string                           `json:"alias"`
	SourceAlias             string                           `json:"sourceAlias"`
	SourcePath              string                           `json:"sourcePath"`
	Operation               sshcSFTP.RemoteTransferOperation `json:"operation"`
	Overwrite               bool                             `json:"overwrite"`
	Direction               sshcSFTP.TransferDirection       `json:"direction"`
	Kind                    sshcSFTP.TransferKind            `json:"kind"`
	Name                    string                           `json:"name"`
	RemotePath              string                           `json:"remotePath"`
	TotalBytes              int64                            `json:"totalBytes"`
	LastModified            int64                            `json:"lastModified"`
	LargeFileThresholdBytes int64                            `json:"largeFileThresholdBytes,omitempty"`
	LargeFileParallelism    int                              `json:"largeFileParallelism,omitempty"`
	LargeFileChunkBytes     int64                            `json:"largeFileChunkBytes,omitempty"`
}

type sftpTransferJobActionRequest struct {
	Action           sshcSFTP.TransferJobAction `json:"action"`
	TransferredBytes *int64                     `json:"transferredBytes,omitempty"`
	TotalBytes       *int64                     `json:"totalBytes,omitempty"`
	Problem          string                     `json:"problem,omitempty"`
	ResetProgress    bool                       `json:"resetProgress,omitempty"`
}

type sftpDownloadCheckpointRequest struct {
	Offset   int64  `json:"offset"`
	Revision string `json:"revision"`
}

// SFTPResumableUpload は、再開できるアップロードの、engine が受け取った位置である。
type SFTPResumableUpload struct {
	ID               string                 `json:"id"`
	Path             string                 `json:"path"`
	Offset           int64                  `json:"offset"`
	Size             int64                  `json:"size"`
	ExpectedRevision string                 `json:"expectedRevision"`
	CompletedRanges  []sshcSFTP.UploadRange `json:"completedRanges"`
	Parallelism      int                    `json:"parallelism"`
	ChunkBytes       int64                  `json:"chunkBytes"`
}

type sftpStartUploadRequest struct {
	Path              string `json:"path"`
	Size              int64  `json:"size"`
	SourceFingerprint string `json:"sourceFingerprint"`
}

type sftpCompleteUploadRequest struct {
	Path              string `json:"path"`
	Size              int64  `json:"size"`
	ExpectedRevision  string `json:"expectedRevision"`
	SourceFingerprint string `json:"sourceFingerprint"`
}

func describeResumableUpload(upload sshcSFTP.ResumableUpload) SFTPResumableUpload {
	ranges := append([]sshcSFTP.UploadRange(nil), upload.CompletedRanges...)
	if ranges == nil {
		ranges = []sshcSFTP.UploadRange{}
	}
	return SFTPResumableUpload{
		ID: upload.ID, Path: upload.Path, Offset: upload.Offset, Size: upload.Size,
		ExpectedRevision: upload.ExpectedRevision, CompletedRanges: ranges,
		Parallelism: upload.Parallelism, ChunkBytes: upload.ChunkBytes,
	}
}
