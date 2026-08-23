package application

import (
	"errors"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"sshc/internal/config"
	"sshc/internal/keys"
	"sshc/internal/storage"
)

var (
	// ErrGroupExists は、既に存在するグループへの名前変更を拒否する。
	ErrGroupExists = errors.New("a group of that name already exists")
	// ErrGroupSelfNesting は、グループを自分自身の中へ移動することを拒否する。
	ErrGroupSelfNesting = errors.New("a group cannot be nested inside itself")
)

// NoticeGroupDirectoryLeftover は、名前変更後に削除できなかったディレクトリを報告する。
const NoticeGroupDirectoryLeftover = "group_directory_leftover"

func (s *Service) RenameGroup(inventory *keys.Inventory, from, to string) (SaveResult, error) {
	s.saveMutex.Lock()
	defer s.saveMutex.Unlock()
	return s.commitGroupPlan(func(graph *config.Graph) (planned, error) {
		return s.planGroupRename(graph, inventory, from, to)
	})
}

func (s *Service) DeleteGroup(inventory *keys.Inventory, name, destination string) (SaveResult, error) {
	s.saveMutex.Lock()
	defer s.saveMutex.Unlock()
	return s.commitGroupPlan(func(graph *config.Graph) (planned, error) {
		return s.planGroupDelete(graph, inventory, name, destination)
	})
}

// commitGroupPlan は、1 つのグループプランを保存と同じコミット経路に通す。
func (s *Service) commitGroupPlan(plan func(*config.Graph) (planned, error)) (SaveResult, error) {
	graph, err := s.resolve()
	if err != nil {
		return SaveResult{}, err
	}
	prepared, err := plan(graph)
	if err != nil {
		return SaveResult{}, err
	}
	if err := s.metadata.EnsureDirectory(); err != nil {
		return SaveResult{}, err
	}
	result, err := s.commitPlannedRequest(prepared, s.requestFor(prepared))
	if err != nil {
		return SaveResult{}, err
	}
	written := make([]string, 0, len(result.Written))
	for _, path := range result.Written {
		written = append(written, s.displayPath(path))
	}
	return SaveResult{
		TransactionID:  result.ID,
		Written:        written,
		Preview:        prepared.preview,
		KeyRelocations: prepared.keyRelocations,
	}, nil
}

// groupRelocation は、グループ操作が移動する 1 つのファイルである。
type groupRelocation struct {
	from string
	to   string
}

func (s *Service) planGroupRename(graph *config.Graph, inventory *keys.Inventory, from, to string) (planned, error) {
	if err := ValidateGroupName(from); err != nil {
		return planned{}, err
	}
	if err := ValidateGroupName(to); err != nil {
		return planned{}, err
	}
	if from == to {
		return planned{}, ErrKeyRelocateUnchanged
	}
	if strings.HasPrefix(to+"/", from+"/") {
		return planned{}, ErrGroupSelfNesting
	}

	declared := s.declaredGroups(graph)
	renamed := make(map[string]string)
	for _, name := range declared {
		switch {
		case name == from:
			renamed[name] = to
		case strings.HasPrefix(name, from+"/"):
			renamed[name] = to + strings.TrimPrefix(name, from)
		case name == to || strings.HasPrefix(name, to+"/"):
			return planned{}, ErrGroupExists
		}
	}
	if len(renamed) == 0 {
		return planned{}, ErrGroupNotDeclared
	}

	next := make([]string, 0, len(declared))
	for _, name := range declared {
		if replacement, ok := renamed[name]; ok {
			next = append(next, replacement)
			continue
		}
		next = append(next, name)
	}
	return s.planGroupLayout(graph, inventory, "config.group_rename", renamed, next, false)
}

func (s *Service) planGroupDelete(graph *config.Graph, inventory *keys.Inventory, name, destination string) (planned, error) {
	if err := ValidateGroupName(name); err != nil {
		return planned{}, err
	}
	if destination != "" {
		if err := ValidateGroupName(destination); err != nil {
			return planned{}, err
		}
		if strings.HasPrefix(destination+"/", name+"/") {
			return planned{}, ErrGroupSelfNesting
		}
	}

	declared := s.declaredGroups(graph)
	removed := make(map[string]bool)
	next := make([]string, 0, len(declared))
	for _, candidate := range declared {
		if candidate == name || strings.HasPrefix(candidate, name+"/") {
			removed[candidate] = true
			continue
		}
		next = append(next, candidate)
	}
	if len(removed) == 0 {
		return planned{}, ErrGroupNotDeclared
	}
	if destination != "" {
		found := false
		for _, candidate := range next {
			if candidate == destination {
				found = true
			}
		}
		if !found {
			return planned{}, ErrGroupNotDeclared
		}
	}

	moved := make(map[string]string, len(removed))
	for candidate := range removed {
		moved[candidate] = destination
	}
	return s.planGroupLayout(graph, inventory, "config.group_delete", moved, next, true)
}

// planGroupLayout は、グループの名前変更や削除に必要な 1 つのトランザクションを構築する。
func (s *Service) planGroupLayout(
	graph *config.Graph,
	inventory *keys.Inventory,
	operation string,
	renamed map[string]string,
	next []string,
	discardPresentation bool,
) (planned, error) {
	layout := &groupLayout{
		service: s, graph: graph, inventory: inventory,
		root: s.workspace.Root(), renamed: renamed, next: next,
		discardPresentation: discardPresentation,
		prepared: planned{
			operation: operation,
			base:      map[string][]byte{},
			baseline:  diagnosticBaseline(graph),
			preview:   SavePreview{Operation: operation},
		},
	}
	if err := layout.stage(); err != nil {
		return planned{}, err
	}
	return layout.prepared, nil
}

type groupLayout struct {
	service   *Service
	graph     *config.Graph
	inventory *keys.Inventory
	root      string
	renamed   map[string]string
	// next は、region がその後に宣言すべきグループ集合である。
	next                []string
	discardPresentation bool

	prepared             planned
	updated              Metadata
	metadataPrecondition storage.Precondition
	connectionMoves      []groupRelocation
	keyMoves             []groupRelocation
	entryFile            *config.File
	entryUpdated         []byte
}

// stage は、全局面を順に通す。並びに意味がある。
func (g *groupLayout) stage() error {
	for _, phase := range []func() error{
		g.stageMoves, g.rewriteMovedKeyReferences, g.stageEntryRegion,
		g.stageGroupSettings, g.stageMetadata, g.noteLeftovers,
	} {
		if err := phase(); err != nil {
			return err
		}
	}
	return nil
}

// stageMoves は、移動するファイルを決め、metadata の identity を追従させる。
func (g *groupLayout) stageMoves() error {
	connectionMoves, err := g.service.groupFileMoves(g.renamed, ConnectionsDirectory, GroupDirectory)
	if err != nil {
		return err
	}
	g.connectionMoves = connectionMoves
	keyMoves, err := g.service.groupFileMoves(g.renamed, KeysDirectory, GroupKeyDirectory)
	if err != nil {
		return err
	}
	g.keyMoves = keyMoves
	for _, relocation := range g.keyMoves {
		g.prepared.keyRelocations = append(g.prepared.keyRelocations, RelocatedKeyFile{
			From: relocation.from,
			To:   relocation.to,
		})
	}

	stored, metadataPrecondition, err := g.service.metadata.Load()
	if err != nil {
		return err
	}
	g.metadataPrecondition = metadataPrecondition
	g.updated = stored
	for _, relocation := range append(append([]groupRelocation{}, g.connectionMoves...), g.keyMoves...) {
		absoluteFrom := filepath.Join(g.root, filepath.FromSlash(relocation.from))
		absoluteTo := filepath.Join(g.root, filepath.FromSlash(relocation.to))
		if _, statErr := g.service.workspace.FileSystem().Lstat(absoluteTo); statErr == nil {
			return ErrGroupExists
		} else if !errors.Is(statErr, fs.ErrNotExist) {
			return statErr
		}
		contents, readErr := g.service.workspace.FileSystem().ReadFile(absoluteFrom)
		if readErr != nil {
			return readErr
		}
		digest := storage.Digest(contents)
		keys.Wipe(contents)
		g.prepared.moves = append(g.prepared.moves, storage.Move{
			From:         absoluteFrom,
			To:           absoluteTo,
			Precondition: storage.Precondition{Exists: true, Digest: digest},
		})
		directory, dirErr := AbsolutePath(g.root, path.Dir(relocation.to))
		if dirErr != nil {
			return dirErr
		}
		g.prepared.directories = append(g.prepared.directories, directory)
	}
	for _, relocation := range g.connectionMoves {
		g.updated = RelocateHostIdentities(g.updated, relocation.from, relocation.to)
	}
	g.updated.Groups = renameGroupMetadata(g.updated.Groups, g.renamed, g.discardPresentation)
	return nil
}

// rewriteMovedKeyReferences は、移動する鍵を指す IdentityFile 行を書き換える。
func (g *groupLayout) rewriteMovedKeyReferences() error {
	keyRelocations := make([]keyRelocation, 0, len(g.keyMoves))
	members := make([]keys.Item, 0, len(g.keyMoves))
	for _, relocation := range g.keyMoves {
		item, found := g.inventory.Find(keys.ItemID(relocation.from))
		if !found {
			continue
		}
		members = append(members, *item)
		keyRelocations = append(keyRelocations, keyRelocation{from: relocation.from, to: relocation.to})
	}
	if blockers := g.service.keyRelocationBlockers(g.graph, g.inventory, members, keyRelocations, "", false); len(blockers) > 0 {
		return &GroupBlockedError{Blockers: blockers}
	}
	changes, _, err := g.service.rewriteKeyReferences(members, keyRelocations)
	if err != nil {
		return err
	}
	for _, change := range changes {
		previous, _, readErr := g.service.readFile(change.Path)
		if readErr != nil {
			return readErr
		}
		g.prepared.changes = append(g.prepared.changes, change)
		g.prepared.base[filepath.Clean(change.Path)] = previous
		g.prepared.preview.Diffs = append(g.prepared.preview.Diffs,
			BuildFileDiff(g.service.displayPath(change.Path), previous, change.Contents))
	}
	return nil
}

// stageEntryRegion は、エントリファイルの生成領域を、これから宣言されるグループで書き直す。
func (g *groupLayout) stageEntryRegion() error {
	entryContents, entryExists, err := g.service.readFile(g.service.entryPath)
	if err != nil {
		return err
	}
	g.entryFile = config.Parse(entryContents)
	regionPlan, err := PlanRegion(g.entryFile, GroupNameOrder(g.next, groupOrder(g.updated)), g.updated.GroupsPath())
	if err != nil {
		return err
	}
	if err := ApplyRegion(g.entryFile, regionPlan); err != nil {
		return err
	}
	g.entryUpdated = g.entryFile.Render()
	entryPrecondition := storage.Precondition{}
	if entryExists {
		entryPrecondition = storage.Precondition{Exists: true, Digest: storage.Digest(entryContents)}
	}
	g.prepared.changes = append(g.prepared.changes, storage.Change{
		Path: g.service.entryPath, Contents: g.entryUpdated, Precondition: entryPrecondition,
	})
	g.prepared.base[filepath.Clean(g.service.entryPath)] = entryContents
	g.prepared.preview.Diffs = append(g.prepared.preview.Diffs,
		BuildFileDiff(entryFileName, diskOrNil(entryContents, entryExists), g.entryUpdated))
	return nil
}

// stageGroupSettings は、groups.sshc.conf を、このトランザクションが生む layout から再生成する。
func (g *groupLayout) stageGroupSettings() error {
	pending := map[string][]byte{filepath.Clean(g.service.entryPath): g.entryUpdated}
	gone := map[string]bool{}
	for _, move := range g.prepared.moves {
		pending[filepath.Clean(move.To)] = nil
		gone[filepath.Clean(move.From)] = true
	}
	for _, move := range g.prepared.moves {
		contents, readErr := g.service.workspace.FileSystem().ReadFile(move.From)
		if readErr != nil {
			return readErr
		}
		pending[filepath.Clean(move.To)] = contents
	}
	after, err := g.service.resolveOverlay(pending, gone)
	if err != nil {
		return err
	}
	hosts, _ := ProjectHosts(after, g.root)
	groupsRelative := g.updated.GroupsPath()
	groupsAbsolute, err := AbsolutePath(g.root, groupsRelative)
	if err != nil {
		return err
	}
	previousGroups, groupsExist, err := g.service.readFile(groupsAbsolute)
	if err != nil {
		return err
	}
	groupContents, groupNotices := CompileGroups(g.next, g.updated, hosts, dominantEnding(g.entryFile))
	g.prepared.preview.Notices = append(g.prepared.preview.Notices, groupNotices...)
	groupsPrecondition := storage.Precondition{}
	if groupsExist {
		groupsPrecondition = storage.Precondition{Exists: true, Digest: storage.Digest(previousGroups)}
	}
	g.prepared.changes = append(g.prepared.changes, storage.Change{
		Path: groupsAbsolute, Contents: groupContents, Precondition: groupsPrecondition,
	})
	g.prepared.base[filepath.Clean(groupsAbsolute)] = previousGroups
	g.prepared.preview.Diffs = append(g.prepared.preview.Diffs,
		BuildFileDiff(groupsRelative, diskOrNil(previousGroups, groupsExist), groupContents))
	return nil
}

// stageMetadata は、metadata の変更をトランザクションへ載せる。
func (g *groupLayout) stageMetadata() error {
	metadataChange, err := g.service.metadata.Change(g.updated, g.metadataPrecondition)
	if err != nil {
		return err
	}
	previousMetadata, _, err := g.service.readFile(metadataChange.Path)
	if err != nil {
		return err
	}
	g.prepared.changes = append(g.prepared.changes, metadataChange)
	g.prepared.base[filepath.Clean(metadataChange.Path)] = previousMetadata
	g.prepared.preview.Diffs = append(g.prepared.preview.Diffs,
		BuildFileDiff(g.service.displayPath(metadataChange.Path), previousMetadata, metadataChange.Contents))
	return nil
}

// noteLeftovers は削除可能なディレクトリと到達不能になる接続を報告する。
func (g *groupLayout) noteLeftovers() error {
	// この操作が空にするディレクトリは、深い方から順に同じトランザクションで一緒に削除される。
	sources := make([]string, 0, len(g.renamed)*2)
	for name := range g.renamed {
		sources = append(sources, GroupDirectory(name), GroupKeyDirectory(name))
	}
	moving := map[string]bool{}
	for _, relocation := range append(append([]groupRelocation{}, g.connectionMoves...), g.keyMoves...) {
		moving[relocation.from] = true
	}
	emptied, left, err := g.service.emptiedDirectories(sources, moving)
	if err != nil {
		return err
	}
	g.prepared.removeDirectories = append(g.prepared.removeDirectories, emptied...)
	for _, directory := range left {
		name := strings.TrimPrefix(strings.TrimPrefix(directory, ConnectionsDirectory+"/"), KeysDirectory+"/")
		g.prepared.preview.Notices = appendNotice(g.prepared.preview.Notices, Notice{
			Code: NoticeGroupDirectoryLeftover, Detail: name, Path: directory,
		})
	}
	for _, relocation := range g.connectionMoves {
		if _, inGroup := GroupOfPath(relocation.to); inGroup {
			continue
		}
		g.prepared.preview.Notices = appendNotice(g.prepared.preview.Notices, Notice{
			Code: NoticeGroupFileUnreached, Path: relocation.to, Detail: relocation.from,
		})
	}
	return nil
}

// GroupBlockedError は、グループ操作が拒否した理由を報告する。
type GroupBlockedError struct {
	Blockers []string
}

func (e *GroupBlockedError) Error() string { return "group operation blocked" }

func (s *Service) groupFileMoves(renamed map[string]string, root string, directoryOf func(string) string) ([]groupRelocation, error) {
	names := make([]string, 0, len(renamed))
	for name := range renamed {
		names = append(names, name)
	}
	sort.Strings(names)

	moves := make([]groupRelocation, 0)
	for _, name := range names {
		source := directoryOf(name)
		absolute := filepath.Join(s.workspace.Root(), filepath.FromSlash(source))
		entries, err := s.workspace.FileSystem().ReadDir(absolute)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		destination := renamed[name]
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			target := root + "/" + entry.Name()
			if destination != "" {
				target = directoryOf(destination) + "/" + entry.Name()
			}
			moves = append(moves, groupRelocation{from: source + "/" + entry.Name(), to: target})
		}
	}
	return moves, nil
}

func renameGroupMetadata(groups []GroupMetadata, renamed map[string]string, discard bool) []GroupMetadata {
	updated := make([]GroupMetadata, 0, len(groups))
	for _, group := range groups {
		destination, affected := renamed[group.Name]
		if !affected {
			updated = append(updated, group)
			continue
		}
		if discard || destination == "" {
			continue
		}
		group.Name = destination
		updated = append(updated, group)
	}
	return updated
}

func groupOrder(metadata Metadata) map[string]int {
	order := make(map[string]int, len(metadata.Groups))
	for _, group := range metadata.Groups {
		order[group.Name] = group.Order
	}
	return order
}

func (s *Service) resolveOverlay(pending map[string][]byte, gone map[string]bool) (*config.Graph, error) {
	resolver := s.resolver
	resolver.Loader = overlayLoader{base: s.resolver.Loader, pending: pending, gone: gone}
	return resolver.Resolve(s.entryPath)
}

func (s *Service) emptiedDirectories(sources []string, moving map[string]bool) (removable, left []string, err error) {
	root := s.workspace.Root()
	ours := map[string]bool{}
	for _, source := range sources {
		ours[source] = true
	}
	seen := map[string]bool{}
	var visit func(relative string) (bool, error)
	visit = func(relative string) (bool, error) {
		absolute := filepath.Join(root, filepath.FromSlash(relative))
		entries, readErr := s.workspace.FileSystem().ReadDir(absolute)
		if errors.Is(readErr, fs.ErrNotExist) {
			return false, nil
		}
		if readErr != nil {
			return false, readErr
		}
		empties := true
		for _, entry := range entries {
			child := relative + "/" + entry.Name()
			if !entry.IsDir() {
				// このトランザクションが移動させないファイルは、ディレクトリを存続させる。
				if !moving[child] {
					empties = false
				}
				continue
			}
			if !ours[child] {
				empties = false
				continue
			}
			childEmpties, childErr := visit(child)
			if childErr != nil {
				return false, childErr
			}
			if !childEmpties {
				empties = false
			}
		}
		if !empties {
			return false, nil
		}
		if !seen[relative] {
			seen[relative] = true
			// 深い順から: これは子が追加され終えた後に実行される。
			removable = append(removable, relative)
		}
		return true, nil
	}

	sort.Strings(sources)
	for _, source := range sources {
		empties, visitErr := visit(source)
		if visitErr != nil {
			return nil, nil, visitErr
		}
		if !empties {
			absolute := filepath.Join(root, filepath.FromSlash(source))
			if _, statErr := s.workspace.FileSystem().Lstat(absolute); statErr == nil {
				left = append(left, source)
			}
		}
	}
	sort.Strings(left)
	return removable, left, nil
}
