package application

// このファイルの型は、handler がそのまま JSON にして返す API の契約である。
// internal/acceptance/contract_drift_test.go が api/openapi.yaml と照合する。

// EditKind は UI が要求できる操作を識別する。
type EditKind string

const (
	EditHostFields      EditKind = "host_fields"
	EditBlockRaw        EditKind = "block_raw"
	EditFileRaw         EditKind = "file_raw"
	EditRename          EditKind = "rename"
	EditDuplicate       EditKind = "duplicate"
	EditGroups          EditKind = "groups"
	EditMetadata        EditKind = "metadata"
	EditMove            EditKind = "move"
	EditComment         EditKind = "comment"
	EditFileRename      EditKind = "file_rename"
	EditFileDelete      EditKind = "file_delete"
	EditDirectoryCreate EditKind = "directory_create"
	EditDirectoryDelete EditKind = "directory_delete"
)

// EditRequest は、要求された 1 個の変更である。
type EditRequest struct {
	Kind     EditKind    `json:"kind"`
	Path     string      `json:"path,omitempty"`
	Base     string      `json:"base,omitempty"`
	Alias    string      `json:"alias,omitempty"`
	NewAlias string      `json:"newAlias,omitempty"`
	Fields   []FieldEdit `json:"fields,omitempty"`
	Raw      string      `json:"raw,omitempty"`
	Comment  string      `json:"comment,omitempty"`
	// HostMetadata と HostMetadataBase は kind "metadata" の、Path と Alias が指す
	// 接続 1 件の新しい metadata と、画面が読み込んだときのその metadata である。
	// HostMetadata が nil なら entry を消し、HostMetadataBase が nil なら画面は
	// entry を見ていない。
	HostMetadata     *HostMetadata `json:"hostMetadata,omitempty"`
	HostMetadataBase *HostMetadata `json:"hostMetadataBase,omitempty"`
	// Groups と GroupsBase は kind "groups" の、新しいグループの設定と、画面が
	// 読み込んだときのグループの設定である。
	Groups           []GroupMetadata `json:"groups,omitempty"`
	GroupsBase       []GroupMetadata `json:"groupsBase,omitempty"`
	DestinationGroup string          `json:"destinationGroup,omitempty"`
	// DestinationPath と DestinationBase は、move の 2 番目のファイルを記述する。
	DestinationPath string `json:"destinationPath,omitempty"`
	DestinationBase string `json:"destinationBase,omitempty"`
}

// SavePreview は、save が書き込むであろうものそのものである。
type SavePreview struct {
	Operation string          `json:"operation"`
	Diffs     []FileDiff      `json:"diffs"`
	Effective []EffectiveDiff `json:"effective,omitempty"`
	Notices   []Notice        `json:"notices,omitempty"`
}

// SaveResult は、commit された transaction を報告する。
type SaveResult struct {
	TransactionID string      `json:"transactionId"`
	Written       []string    `json:"written"`
	Preview       SavePreview `json:"preview"`
}

// IncludeReference は、1 個の Include 引数と、それが解決した先である。
type IncludeReference struct {
	Line      int       `json:"line"`
	Pattern   string    `json:"pattern"`
	Condition string    `json:"condition,omitempty"`
	Matches   []FileRef `json:"matches,omitempty"`
}

// FileNode は、Include graph の 1 個のファイルである。
type FileNode struct {
	File     FileRef            `json:"file"`
	Missing  bool               `json:"missing,omitempty"`
	Editable bool               `json:"editable"`
	Loads    int                `json:"loads"`
	Includes []IncludeReference `json:"includes,omitempty"`
}

// FileContents は、raw editor のための設定ファイル全体である。
type FileContents struct {
	File     FileRef `json:"file"`
	Contents string  `json:"contents"`
	Digest   string  `json:"digest"`
	Editable bool    `json:"editable"`
	Exists   bool    `json:"exists"`
}

// PendingView は、ユーザーが判断しなければならない中断された transaction である。
type PendingView struct {
	ID          string   `json:"id"`
	Operation   string   `json:"operation"`
	Status      string   `json:"status"`
	StartedAt   string   `json:"startedAt"`
	Committed   int      `json:"committed"`
	Paths       []string `json:"paths"`
	CanComplete bool     `json:"canComplete"`
}

// HistoryEntry は、1 個の完了した transaction である。
type HistoryEntry struct {
	ID         string   `json:"id"`
	Operation  string   `json:"operation"`
	Status     string   `json:"status"`
	StartedAt  string   `json:"startedAt"`
	FinishedAt string   `json:"finishedAt,omitempty"`
	Paths      []string `json:"paths"`
	Restorable []string `json:"restorable,omitempty"`
}

// Overview は、Connections tree と Config Explorer が必要とするすべてである。
type Overview struct {
	Entry       FileRef          `json:"entry"`
	Files       []FileNode       `json:"files"`
	Hosts       []HostEntry      `json:"hosts"`
	Groups      []GroupView      `json:"groups"`
	Metadata    Metadata         `json:"metadata"`
	Diagnostics []DiagnosticView `json:"diagnostics"`
	Notices     []Notice         `json:"notices"`
	Pending     []PendingView    `json:"pending,omitempty"`
}

type HostDetail struct {
	Form      HostForm     `json:"form"`
	Metadata  HostMetadata `json:"metadata"`
	Effective Effective    `json:"effective"`
	File      FileContents `json:"file"`
}
