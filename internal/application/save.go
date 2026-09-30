package application

import (
	"errors"
	"path"
	"path/filepath"

	"sshc/internal/config"
	"sshc/internal/effective"
	"sshc/internal/storage"
)

// maxEffectivePreviews は、1 回の保存の preview に載せる実効値の差分の上限。
const maxEffectivePreviews = 50

var (
	ErrUnknownEditKind  = errors.New("unknown edit kind")
	ErrGroupNotDeclared = errors.New("no generated Include line declares that group")
	// ErrAmbiguousDestination は、グループと path の両方を指定した move を拒否する。
	ErrAmbiguousDestination = errors.New("a move names either a destination group or a destination path")
)

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
	// derived from the resulting concrete Host block, never from inherited
	// effective configuration.
	explicitIdentityFile bool
	// passwordAuthenticationOff is evaluated against the resulting graph so a
	// request that removes a direct key can assign a password in the same save.
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

// hostBlockMutations は、Host ブロックひとつを書き換える種別である。
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
	EditDuplicate: func(graph *config.Graph, file *config.File, _ config.Block, request EditRequest) error {
		if err := refuseTakenAlias(graph, "", request.NewAlias); err != nil {
			return err
		}
		return DuplicateHostBlock(file, request.Alias, request.NewAlias)
	},
}

func (s *Service) plan(request EditRequest) (planned, error) {
	graph, err := s.resolve()
	if err != nil {
		return planned{}, err
	}
	prepared, err := s.planEdit(graph, request)
	if err != nil || request.Kind == EditGroups || request.Kind == EditMetadata {
		return prepared, err
	}
	metadata, err := s.plannedMetadata(prepared)
	if err != nil {
		return planned{}, err
	}
	after, err := s.refreshGroupSettings(&prepared, metadata)
	if err != nil {
		return planned{}, err
	}
	if request.Alias != "" {
		alias := request.Alias
		if request.Kind == EditRename || request.Kind == EditDuplicate {
			alias = request.NewAlias
		}
		prepared.preview.Effective = []EffectiveDiff{DiffEffective(
			ComputeEffective(graph, s.workspace.Root(), request.Alias, s.localFacts()),
			ComputeEffective(after, s.workspace.Root(), alias, s.localFacts()),
		)}
	}
	return prepared, nil
}

func (s *Service) planEdit(graph *config.Graph, request EditRequest) (planned, error) {
	switch request.Kind {
	case EditHostFields, EditBlockRaw, EditRename, EditDuplicate, EditFileRaw, EditComment:
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

// refuseTakenAlias は、Include graph が届くどこかで別の Host ブロックが既に宣言している
// 名前への変更を断る。
//
// graph は編集の前に読んだものなので、名前を変えるブロックはまだ古い名前を持ち、新しい
// 名前とは一致しない。今の名前と同じ名前への変更は何も変えないので、自分自身との衝突として
// 断らずに通す。
func refuseTakenAlias(graph *config.Graph, from, to string) error {
	if from == to {
		return nil
	}
	var taken error
	WalkDirectives(graph, func(visit Visit) bool {
		if visit.Block.Kind != config.BlockHost || visit.Block.Header != visit.Index {
			return true
		}
		if effective.DeclaresExactly(visit.Block.Patterns, to) {
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
