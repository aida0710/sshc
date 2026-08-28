package application

import (
	"errors"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"sshc/internal/config"
	"sshc/internal/effective"
	"sshc/internal/storage"
	"sshc/internal/terminal"
)

const (
	entryFileName        = "config"
	maxEffectivePreviews = 50
)

var (
	ErrUnknownEditKind       = errors.New("unknown edit kind")
	ErrUnknownRecoveryAction = errors.New("unknown recovery action")
	ErrNotEditable           = errors.New("file is not editable through this application")
	ErrGroupNotDeclared      = errors.New("no generated Include line declares that group")
	// ErrAmbiguousDestination は、グループと path の両方を指定した move を拒否する。
	ErrAmbiguousDestination = errors.New("a move names either a destination group or a destination path")
)

// EditKind は UI が要求できる操作を識別する。
type EditKind string

const (
	EditHostFields      EditKind = "host_fields"
	EditBlockRaw        EditKind = "block_raw"
	EditFileRaw         EditKind = "file_raw"
	EditRename          EditKind = "rename"
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
	Kind             EditKind    `json:"kind"`
	Path             string      `json:"path,omitempty"`
	Base             string      `json:"base,omitempty"`
	Alias            string      `json:"alias,omitempty"`
	NewAlias         string      `json:"newAlias,omitempty"`
	Fields           []FieldEdit `json:"fields,omitempty"`
	Raw              string      `json:"raw,omitempty"`
	Comment          string      `json:"comment,omitempty"`
	Metadata         *Metadata   `json:"metadata,omitempty"`
	DestinationGroup string      `json:"destinationGroup,omitempty"`
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
	TransactionID  string             `json:"transactionId"`
	Written        []string           `json:"written"`
	Preview        SavePreview        `json:"preview"`
	KeyRelocations []RelocatedKeyFile `json:"-"`
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

type Service struct {
	workspace *storage.Workspace
	manager   *storage.Manager
	resolver  config.Resolver
	metadata  *MetadataStore
	entryPath string

	saveMutex       sync.Mutex
	pendingBase     map[string][]byte
	pendingBaseline map[string]bool
	keyPassphrases  KeyPassphraseVerifier
}

func resolverFor(workspace *storage.Workspace) config.Resolver {
	resolver := storage.NewResolver(workspace)
	resolver.GeneratedRegion = GeneratedRegion
	return resolver
}

func NewService(workspace *storage.Workspace, manager *storage.Manager) *Service {
	service := &Service{
		workspace: workspace,
		manager:   manager,
		resolver:  resolverFor(workspace),
		metadata:  NewMetadataStore(workspace),
		entryPath: filepath.Join(workspace.Root(), entryFileName),
	}
	manager.Validate = service.validate
	return service
}

// TerminalLimits は、埋め込みターミナルが開くたびに読む上限を返す。
func (s *Service) TerminalLimits() terminal.Limits {
	metadata, _, err := s.metadata.Load()
	if err != nil {
		return terminal.DefaultLimits()
	}
	return metadata.TerminalLimits()
}

func (s *Service) displayPath(absolute string) string {
	reference := NewFileRef(s.workspace.Root(), absolute)
	if reference.External {
		return reference.Absolute
	}
	return reference.Path
}

func (s *Service) readFile(absolute string) (contents []byte, exists bool, err error) {
	contents, err = s.workspace.FileSystem().ReadFile(absolute)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return contents, true, nil
}

// localFacts は、トークン展開に要るこのプロセスの事実である。
func (s *Service) localFacts() effective.LocalFacts {
	return LocalFactsFor(s.workspace.Home())
}

func (s *Service) resolve() (*config.Graph, error) {
	return s.resolver.Resolve(s.entryPath)
}

func (s *Service) resolveWith(pending map[string][]byte) (*config.Graph, error) {
	resolver := s.resolver
	resolver.Loader = overlayLoader{base: s.resolver.Loader, pending: pending}
	return resolver.Resolve(s.entryPath)
}

// Overview は、Connections tree、Include graph、metadata を構築する。
func (s *Service) Overview() (Overview, error) {
	graph, err := s.resolve()
	if err != nil {
		return Overview{}, err
	}
	root := s.workspace.Root()
	hosts, notices := ProjectHosts(graph, root)
	facts := s.localFacts()
	for index := range hosts {
		alias := hosts[index].Identity.Alias
		if alias == "" {
			continue
		}
		resolution := effective.Resolve(graph, alias, facts)
		if len(resolution.Refusals) != 0 {
			continue
		}
		hosts[index].HostName = resolution.Values.First("hostname")
		hosts[index].User = resolution.Values.First("user")
		hosts[index].Port = resolution.Values.First("port")
	}

	stored, _, err := s.metadata.Load()
	if err != nil {
		return Overview{}, err
	}
	identities := make([]HostIdentity, 0, len(hosts))
	for _, host := range hosts {
		if !host.Identity.IsZero() {
			identities = append(identities, host.Identity)
		}
	}
	reconciled, orphanNotices := ReconcileMetadata(stored, identities)
	notices = append(notices, orphanNotices...)
	notices = append(notices, s.unreachedConnectionFiles(graph)...)

	entryNode := graph.Nodes[s.entryPath]
	var groups []GroupView
	if entryNode != nil && entryNode.File != nil {
		if _, _, _, regionErr := FindRegion(entryNode.File); errors.Is(regionErr, ErrRegionDamaged) {
			notices = append(notices, Notice{
				Code: NoticeRegionDamaged, Path: s.displayPath(s.entryPath),
			})
		} else {
			present, presentErr := s.presentGroupDirectories()
			if presentErr != nil {
				return Overview{}, presentErr
			}
			var groupNotices []Notice
			groups, groupNotices = BuildGroupsView(entryNode.File, hosts, reconciled, present)
			notices = append(notices, groupNotices...)
		}
	}

	overview := Overview{
		Entry:    NewFileRef(root, s.entryPath),
		Hosts:    hosts,
		Groups:   groups,
		Metadata: reconciled,
		Notices:  notices,
	}
	for _, nodePath := range graph.Order {
		node := graph.Nodes[nodePath]
		reference := NewFileRef(root, nodePath)
		file := FileNode{
			File:     reference,
			Missing:  node.Missing,
			Editable: node.Editable && !reference.External,
			Loads:    node.Loads,
		}
		for _, edge := range node.Includes {
			include := IncludeReference{Line: edge.Line, Pattern: edge.Pattern, Condition: edge.Condition}
			for _, match := range edge.Matches {
				include.Matches = append(include.Matches, NewFileRef(root, match))
			}
			file.Includes = append(file.Includes, include)
		}
		overview.Files = append(overview.Files, file)
	}
	for _, diagnostic := range graph.Diagnostics {
		overview.Diagnostics = append(overview.Diagnostics, NewDiagnosticView(root, diagnostic))
	}
	pending, err := s.Pending()
	if err != nil {
		return Overview{}, err
	}
	overview.Pending = pending

	if overview.Files == nil {
		overview.Files = []FileNode{}
	}
	if overview.Hosts == nil {
		overview.Hosts = []HostEntry{}
	}
	// エントリファイルが無いワークスペースは、ここを nil のまま通る。
	if overview.Groups == nil {
		overview.Groups = []GroupView{}
	}
	if overview.Diagnostics == nil {
		overview.Diagnostics = []DiagnosticView{}
	}
	if overview.Notices == nil {
		overview.Notices = []Notice{}
	}
	return overview, nil
}

// HostDetail は、説明された値と共に 1 個のホストブロックを射影する。
func (s *Service) HostDetail(relative, alias string) (HostDetail, error) {
	graph, err := s.resolve()
	if err != nil {
		return HostDetail{}, err
	}
	identity := HostIdentity{Path: relative, Alias: alias}
	form, err := ProjectHostForm(graph, s.workspace.Root(), identity)
	if err != nil {
		return HostDetail{}, err
	}
	contents, err := s.FileContents(relative)
	if err != nil {
		return HostDetail{}, err
	}
	stored, _, err := s.metadata.Load()
	if err != nil {
		return HostDetail{}, err
	}
	detail := HostDetail{
		Form:      form,
		Effective: ComputeEffective(graph, s.workspace.Root(), alias, s.localFacts()),
		File:      contents,
		Metadata:  HostMetadata{Identity: identity},
	}
	for _, host := range stored.Hosts {
		if host.Identity == identity {
			detail.Metadata = host
		}
	}
	return detail, nil
}

// FileContents は、ワークスペース内の編集可能な 1 個のファイルを読む。
func (s *Service) FileContents(relative string) (FileContents, error) {
	absolute, err := AbsolutePath(s.workspace.Root(), relative)
	if err != nil {
		return FileContents{}, err
	}
	contents, exists, err := s.readFile(absolute)
	if err != nil {
		return FileContents{}, err
	}
	editable := true
	if _, resolveErr := s.workspace.ResolveForWrite(absolute); resolveErr != nil {
		editable = false
	}
	return FileContents{
		File:     NewFileRef(s.workspace.Root(), absolute),
		Contents: string(contents),
		Digest:   storage.Digest(contents),
		Editable: editable,
		Exists:   exists,
	}, nil
}

func (s *Service) requestFor(prepared planned) storage.Request {
	return storage.Request{
		Operation:         prepared.operation,
		Directories:       directoryCreates(prepared.directories),
		Changes:           prepared.changes,
		Moves:             prepared.moves,
		Removals:          prepared.removals,
		RemoveDirectories: directoryRemovals(s.workspace.Root(), prepared.removeDirectories),
	}
}

func directoryCreates(absolute []string) []storage.DirectoryCreate {
	creates := make([]storage.DirectoryCreate, 0, len(absolute))
	seen := map[string]bool{}
	for _, path := range absolute {
		cleaned := filepath.Clean(path)
		if seen[cleaned] {
			continue
		}
		seen[cleaned] = true
		creates = append(creates, storage.DirectoryCreate{Path: cleaned})
	}
	return creates
}

func directoryRemovals(root string, relative []string) []storage.DirectoryRemoval {
	removals := make([]storage.DirectoryRemoval, 0, len(relative))
	seen := map[string]bool{}
	for _, path := range relative {
		absolute := filepath.Join(root, filepath.FromSlash(path))
		if seen[absolute] {
			continue
		}
		seen[absolute] = true
		removals = append(removals, storage.DirectoryRemoval{Path: absolute})
	}
	return removals
}

type planned struct {
	operation string
	changes   []storage.Change
	// explicitIdentityFile is meaningful for a connection.update plan. It is
	explicitIdentityFile bool
	// passwordAuthenticationOff is evaluated against the resulting graph so a
	passwordAuthenticationOff bool
	// authenticationBinding binds a saved account password to the fully resolved
	// destination produced by this plan.
	authenticationBinding string
	moves                 []storage.Move
	removals              []storage.Removal
	directories           []string
	removeDirectories     []string
	base                  map[string][]byte
	baseline              map[string]bool
	preview               SavePreview
	keyRelocations        []RelocatedKeyFile
}

// Preview は transaction を準備し、書き込まずにその diff を返す。
func (s *Service) Preview(request EditRequest) (SavePreview, error) {
	s.saveMutex.Lock()
	defer s.saveMutex.Unlock()
	prepared, err := s.plan(request)
	if err != nil {
		return SavePreview{}, err
	}
	return prepared.preview, nil
}

// Save は同じ transaction を準備し、それを commit する。
func (s *Service) Save(request EditRequest) (SaveResult, error) {
	s.saveMutex.Lock()
	defer s.saveMutex.Unlock()
	prepared, err := s.plan(request)
	if err != nil {
		return SaveResult{}, err
	}
	metadataPath := filepath.Clean(s.metadata.Path())
	for _, change := range prepared.changes {
		if filepath.Clean(change.Path) != metadataPath {
			continue
		}
		if err := s.metadata.EnsureDirectory(); err != nil {
			return SaveResult{}, err
		}
	}
	result, err := s.commitPlannedRequest(prepared, s.requestFor(prepared))
	if err != nil {
		return SaveResult{}, err
	}
	written := make([]string, 0, len(result.Written))
	for _, path := range result.Written {
		written = append(written, s.displayPath(path))
	}
	return SaveResult{TransactionID: result.ID, Written: written, Preview: prepared.preview}, nil
}

// hostBlockMutations は、Host ブロックひとつを書き換える 4 つの種別である。
var hostBlockMutations = map[EditKind]func(*config.Graph, *config.File, config.Block, EditRequest) error{
	EditHostFields: func(_ *config.Graph, file *config.File, block config.Block, request EditRequest) error {
		return ApplyFieldEdits(file, block, request.Fields)
	},
	EditBlockRaw: func(_ *config.Graph, file *config.File, block config.Block, request EditRequest) error {
		return ReplaceBlock(file, block, request.Raw)
	},
	EditComment: func(_ *config.Graph, file *config.File, block config.Block, request EditRequest) error {
		return SetHostComment(file, block, request.Comment)
	},
	EditRename: func(graph *config.Graph, file *config.File, block config.Block, request EditRequest) error {
		if err := refuseTakenAlias(graph, request.Alias, request.NewAlias); err != nil {
			return err
		}
		return RenameHostAlias(file, block, request.Alias, request.NewAlias)
	},
}

func (s *Service) plan(request EditRequest) (planned, error) {
	graph, err := s.resolve()
	if err != nil {
		return planned{}, err
	}
	switch request.Kind {
	case EditHostFields, EditBlockRaw, EditRename, EditFileRaw, EditComment:
		return s.planFileEdit(graph, request)
	case EditGroups, EditMetadata:
		return s.planMetadataEdit(graph, request)
	case EditMove:
		return s.planMoveHost(graph, request)
	case EditFileRename:
		return s.planFileRename(graph, request)
	case EditDirectoryCreate:
		return s.planDirectoryCreate(graph, request)
	case EditDirectoryDelete:
		return s.planDirectoryDelete(graph, request)
	case EditFileDelete:
		return s.planFileDelete(graph, request)
	default:
		return planned{}, ErrUnknownEditKind
	}
}

// resolveDestination は、destination グループを destination path へ変える。
func (s *Service) resolveDestination(graph *config.Graph, request EditRequest) (EditRequest, error) {
	if request.DestinationGroup == "" {
		return request, nil
	}
	if request.DestinationPath != "" {
		return EditRequest{}, ErrAmbiguousDestination
	}
	if err := ValidateGroupName(request.DestinationGroup); err != nil {
		return EditRequest{}, err
	}
	declared := false
	for _, name := range s.declaredGroups(graph) {
		if name == request.DestinationGroup {
			declared = true
			break
		}
	}
	if !declared {
		return EditRequest{}, ErrGroupNotDeclared
	}
	name := GroupFileName(path.Base(filepath.ToSlash(request.Path)))
	request.DestinationPath = GroupDirectory(request.DestinationGroup) + "/" + name
	return request, nil
}

func (s *Service) destinationWillBeRead(graph *config.Graph, absolute, relative string) bool {
	if _, included := graph.Nodes[absolute]; included {
		return true
	}
	destination := filepath.ToSlash(relative)
	for _, name := range s.declaredGroups(graph) {
		if matched, err := path.Match(GroupIncludePattern(name), destination); err == nil && matched {
			return true
		}
	}
	return false
}

func (s *Service) presentGroupDirectories() ([]string, error) {
	root := s.workspace.Root()
	base := filepath.Join(root, ConnectionsDirectory)
	var present []string
	var walk func(directory, prefix string, depth int) error
	walk = func(directory, prefix string, depth int) error {
		if depth > MaxGroupSegments {
			return nil
		}
		entries, err := s.workspace.FileSystem().ReadDir(directory)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if prefix != "" {
				name = prefix + "/" + name
			}
			present = append(present, name)
			if err := walk(filepath.Join(directory, entry.Name()), name, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(base, "", 1); err != nil {
		return nil, err
	}
	sort.Strings(present)
	return present, nil
}

func (s *Service) unreachedConnectionFiles(graph *config.Graph) []Notice {
	root := s.workspace.Root()
	base := filepath.Join(root, ConnectionsDirectory)
	var notices []Notice

	var walk func(directory string, depth int)
	walk = func(directory string, depth int) {
		if depth > MaxGroupSegments {
			return
		}
		entries, err := s.workspace.FileSystem().ReadDir(directory)
		if err != nil {
			return
		}
		for _, entry := range entries {
			path := filepath.Join(directory, entry.Name())
			if entry.IsDir() {
				walk(path, depth+1)
				continue
			}
			if !strings.HasSuffix(entry.Name(), groupFileSuffix) {
				continue
			}
			if _, reached := graph.Nodes[path]; reached {
				continue
			}
			notices = appendNotice(notices, Notice{
				Code: NoticeGroupFileUnreached,
				Path: NewFileRef(root, path).Path,
			})
		}
	}
	walk(base, 0)
	return notices
}

// refuseTakenAlias は、Include graph が届くどこであれ、別の Host
func refuseTakenAlias(graph *config.Graph, from, to string) error {
	if from == to {
		return nil
	}
	var taken error
	WalkDirectives(graph, func(visit Visit) bool {
		if visit.Block.Kind != config.BlockHost || visit.Block.Header != visit.Index {
			return true
		}
		if declaresExactly(visit.Block.Patterns, to) {
			taken = ErrAliasAlreadyDeclared
			return false
		}
		return true
	})
	return taken
}

// declaredGroups は、解決済みのエントリファイルからグループ宣言を読み出す。
func (s *Service) declaredGroups(graph *config.Graph) []string {
	node := graph.Nodes[s.entryPath]
	if node == nil || node.File == nil {
		return nil
	}
	return DeclaredGroups(node.File)
}

func diskOrNil(contents []byte, exists bool) []byte {
	if !exists {
		return nil
	}
	if contents == nil {
		return []byte{}
	}
	return contents
}

func (s *Service) Pending() ([]PendingView, error) {
	pending, err := s.manager.Pending()
	if err != nil {
		return nil, err
	}
	views := make([]PendingView, 0, len(pending))
	for _, item := range pending {
		view := PendingView{
			ID:          item.ID,
			Operation:   item.Operation,
			Status:      item.Status,
			StartedAt:   item.StartedAt.UTC().Format(time.RFC3339),
			Committed:   item.Committed,
			CanComplete: item.CanComplete,
		}
		for _, entry := range item.Entries {
			view.Paths = append(view.Paths, s.displayPath(entry.Path))
		}
		views = append(views, view)
	}
	return views, nil
}

func (s *Service) Recover(identifier, action string) error {
	s.saveMutex.Lock()
	defer s.saveMutex.Unlock()
	switch action {
	case "complete":
		return s.manager.Complete(identifier)
	case "rollback":
		return s.manager.Rollback(identifier)
	default:
		return ErrUnknownRecoveryAction
	}
}

func (s *Service) History() ([]HistoryEntry, error) {
	records, err := s.manager.History()
	if err != nil {
		return nil, err
	}
	entries := make([]HistoryEntry, 0, len(records))
	for _, record := range records {
		entry := HistoryEntry{
			ID:        record.ID,
			Operation: record.Operation,
			Status:    record.Status,
			StartedAt: record.StartedAt.UTC().Format(time.RFC3339),
		}
		if !record.FinishedAt.IsZero() {
			entry.FinishedAt = record.FinishedAt.UTC().Format(time.RFC3339)
		}
		for _, path := range record.Paths {
			display := s.displayPath(path)
			entry.Paths = append(entry.Paths, display)
			relative, relativeErr := RelativePath(s.workspace.Root(), path)
			if relativeErr != nil || record.BackupDir == "" {
				continue
			}
			backup := filepath.Join(record.BackupDir, filepath.FromSlash(relative))
			if _, statErr := s.workspace.FileSystem().Lstat(backup); statErr == nil {
				entry.Restorable = append(entry.Restorable, display)
			}
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (s *Service) Restore(identifier, relative string) (SaveResult, error) {
	s.saveMutex.Lock()
	defer s.saveMutex.Unlock()

	records, err := s.manager.History()
	if err != nil {
		return SaveResult{}, err
	}
	var record storage.HistoryRecord
	found := false
	for _, candidate := range records {
		if candidate.ID == identifier {
			record, found = candidate, true
		}
	}
	if !found {
		return SaveResult{}, storage.ErrUnknownTransaction
	}
	absolute, err := AbsolutePath(s.workspace.Root(), relative)
	if err != nil {
		return SaveResult{}, err
	}
	contents, err := s.manager.ReadBackup(filepath.Join(record.BackupDir, filepath.FromSlash(relative)))
	if err != nil {
		return SaveResult{}, err
	}
	current, exists, err := s.readFile(absolute)
	if err != nil {
		return SaveResult{}, err
	}
	precondition := storage.Precondition{}
	if exists {
		precondition = storage.Precondition{Exists: true, Digest: storage.Digest(current)}
	}
	graph, err := s.resolve()
	if err != nil {
		return SaveResult{}, err
	}

	if err := s.metadata.EnsureDirectory(); err != nil {
		return SaveResult{}, err
	}
	s.pendingBase = map[string][]byte{filepath.Clean(absolute): current}
	s.pendingBaseline = diagnosticBaseline(graph)
	defer func() { s.pendingBase, s.pendingBaseline = nil, nil }()

	result, err := s.manager.Commit(storage.Request{
		Operation: "config.restore",
		Changes:   []storage.Change{{Path: absolute, Contents: contents, Precondition: precondition}},
	})
	if err != nil {
		return SaveResult{}, err
	}
	return SaveResult{
		TransactionID: result.ID,
		Written:       []string{relative},
		Preview: SavePreview{
			Operation: "config.restore",
			Diffs:     []FileDiff{BuildFileDiff(relative, diskOrNil(current, exists), contents)},
		},
	}, nil
}
