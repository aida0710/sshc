package application

import (
	"bytes"
	"path/filepath"

	"sshc/internal/config"
	"sshc/internal/storage"
)

// この一式は、EditRequest ひとつを planned へ落とす。

func (s *Service) planFileEdit(graph *config.Graph, request EditRequest) (planned, error) {
	absolute, err := AbsolutePath(s.workspace.Root(), request.Path)
	if err != nil {
		return planned{}, err
	}
	if _, err := s.workspace.ResolveForWrite(absolute); err != nil {
		return planned{}, err
	}
	base := []byte(request.Base)
	file := config.Parse(base)

	var renameFrom, renameTo HostIdentity
	if request.Kind == EditFileRaw {
		// ファイル全体の置換ではブロックを検索しない。
		file = config.Parse([]byte(request.Raw))
	} else {
		block, ok := FindHostBlock(file, request.Alias)
		if !ok {
			return planned{}, ErrHostNotFound
		}
		mutate, known := hostBlockMutations[request.Kind]
		if !known {
			return planned{}, ErrUnknownEditKind
		}
		if err := mutate(graph, file, block, request); err != nil {
			return planned{}, err
		}
		if request.Kind == EditRename {
			renameFrom = HostIdentity{Path: request.Path, Alias: request.Alias}
			renameTo = HostIdentity{Path: request.Path, Alias: request.NewAlias}
		}
	}
	updated := file.Render()

	disk, exists, err := s.readFile(absolute)
	if err != nil {
		return planned{}, err
	}
	if !bytes.Equal(base, disk) {
		return planned{}, &ConflictError{Report: BuildConflictReport(request.Path, base, disk, updated)}
	}

	precondition := storage.Precondition{}
	if exists {
		precondition = storage.Precondition{Exists: true, Digest: storage.Digest(base)}
	}
	prepared := planned{
		operation: "config." + string(request.Kind),
		changes:   []storage.Change{{Path: absolute, Contents: updated, Precondition: precondition}},
		base:      map[string][]byte{filepath.Clean(absolute): base},
		baseline:  diagnosticBaseline(graph),
		preview: SavePreview{
			Operation: "config." + string(request.Kind),
			Diffs:     []FileDiff{BuildFileDiff(request.Path, diskOrNil(disk, exists), updated)},
		},
	}

	if !renameFrom.IsZero() {
		stored, precondition, err := s.metadata.Load()
		if err != nil {
			return planned{}, err
		}
		renamed := RenameHostIdentity(stored, renameFrom, renameTo)
		change, err := s.metadata.Change(renamed, precondition)
		if err != nil {
			return planned{}, err
		}
		previous, _, err := s.readFile(change.Path)
		if err != nil {
			return planned{}, err
		}
		prepared.changes = append(prepared.changes, change)
		prepared.base[filepath.Clean(change.Path)] = previous
		prepared.preview.Diffs = append(prepared.preview.Diffs,
			BuildFileDiff(s.displayPath(change.Path), previous, change.Contents))
	}

	if request.Kind == EditComment {
		stored, precondition, err := s.metadata.Load()
		if err != nil {
			return planned{}, err
		}
		cleared := ClearHostNote(stored, HostIdentity{Path: request.Path, Alias: request.Alias})
		change, err := s.metadata.Change(cleared, precondition)
		if err != nil {
			return planned{}, err
		}
		previous, _, err := s.readFile(change.Path)
		if err != nil {
			return planned{}, err
		}
		if !bytes.Equal(previous, change.Contents) {
			prepared.changes = append(prepared.changes, change)
			prepared.base[filepath.Clean(change.Path)] = previous
			prepared.preview.Diffs = append(prepared.preview.Diffs,
				BuildFileDiff(s.displayPath(change.Path), previous, change.Contents))
		}
	}

	if request.Alias != "" {
		pending := map[string][]byte{filepath.Clean(absolute): updated}
		after, err := s.resolveWith(pending)
		if err != nil {
			return planned{}, err
		}
		alias := request.Alias
		if request.Kind == EditRename {
			alias = request.NewAlias
		}
		prepared.preview.Effective = []EffectiveDiff{DiffEffective(
			ComputeEffective(graph, s.workspace.Root(), request.Alias, s.localFacts()),
			ComputeEffective(after, s.workspace.Root(), alias, s.localFacts()),
		)}
	}
	return prepared, nil
}

// planMoveHost は、1 個のホストブロックを別のファイルへ移動する。
func (s *Service) planMoveHost(graph *config.Graph, request EditRequest) (planned, error) {
	root := s.workspace.Root()
	request, err := s.resolveDestination(graph, request)
	if err != nil {
		return planned{}, err
	}
	sourceAbsolute, err := AbsolutePath(root, request.Path)
	if err != nil {
		return planned{}, err
	}
	destinationAbsolute, err := AbsolutePath(root, request.DestinationPath)
	if err != nil {
		return planned{}, err
	}
	if sourceAbsolute == destinationAbsolute {
		return planned{}, ErrSameFileMove
	}
	if _, err := s.workspace.ResolveForWrite(sourceAbsolute); err != nil {
		return planned{}, err
	}
	if _, err := s.workspace.ResolveForWrite(destinationAbsolute); err != nil {
		return planned{}, err
	}

	destinationDisk, destinationExists, err := s.readFile(destinationAbsolute)
	if err != nil {
		return planned{}, err
	}

	sourceBase := []byte(request.Base)
	destinationBase := []byte(request.DestinationBase)
	if request.DestinationGroup != "" && request.DestinationBase == "" {
		destinationBase = destinationDisk
	}

	sourceFile := config.Parse(sourceBase)
	destinationFile := config.Parse(destinationBase)
	moved, err := MoveHostBlock(sourceFile, destinationFile, request.Alias)
	if err != nil {
		return planned{}, err
	}
	sourceUpdated := sourceFile.Render()
	destinationUpdated := destinationFile.Render()

	sourceDisk, sourceExists, err := s.readFile(sourceAbsolute)
	if err != nil {
		return planned{}, err
	}
	if !bytes.Equal(sourceBase, sourceDisk) {
		return planned{}, &ConflictError{Report: BuildConflictReport(request.Path, sourceBase, sourceDisk, sourceUpdated)}
	}
	if !bytes.Equal(destinationBase, destinationDisk) {
		return planned{}, &ConflictError{
			Report: BuildConflictReport(request.DestinationPath, destinationBase, destinationDisk, destinationUpdated),
		}
	}

	sourcePrecondition := storage.Precondition{}
	if sourceExists {
		sourcePrecondition = storage.Precondition{Exists: true, Digest: storage.Digest(sourceBase)}
	}
	destinationPrecondition := storage.Precondition{}
	if destinationExists {
		destinationPrecondition = storage.Precondition{Exists: true, Digest: storage.Digest(destinationBase)}
	}

	stored, metadataPrecondition, err := s.metadata.Load()
	if err != nil {
		return planned{}, err
	}
	relocated := RenameHostIdentity(stored,
		HostIdentity{Path: request.Path, Alias: request.Alias},
		HostIdentity{Path: request.DestinationPath, Alias: request.Alias},
	)
	metadataChange, err := s.metadata.Change(relocated, metadataPrecondition)
	if err != nil {
		return planned{}, err
	}
	previousMetadata, _, err := s.readFile(metadataChange.Path)
	if err != nil {
		return planned{}, err
	}

	prepared := planned{
		operation: "config.move",
		changes: []storage.Change{
			{Path: sourceAbsolute, Contents: sourceUpdated, Precondition: sourcePrecondition},
			{Path: destinationAbsolute, Contents: destinationUpdated, Precondition: destinationPrecondition},
			metadataChange,
		},
		base: map[string][]byte{
			filepath.Clean(sourceAbsolute):      sourceBase,
			filepath.Clean(destinationAbsolute): destinationBase,
			filepath.Clean(metadataChange.Path): previousMetadata,
		},
		baseline: diagnosticBaseline(graph),
		preview: SavePreview{
			Operation: "config.move",
			Diffs: []FileDiff{
				BuildFileDiff(request.Path, diskOrNil(sourceDisk, sourceExists), sourceUpdated),
				BuildFileDiff(request.DestinationPath, diskOrNil(destinationDisk, destinationExists), destinationUpdated),
				BuildFileDiff(s.displayPath(metadataChange.Path), previousMetadata, metadataChange.Contents),
			},
		},
	}

	if !s.destinationWillBeRead(graph, destinationAbsolute, request.DestinationPath) {
		prepared.preview.Notices = appendNotice(prepared.preview.Notices, Notice{
			Code:   NoticeDestinationNotIncluded,
			Path:   request.DestinationPath,
			Detail: request.Alias,
		})
	}
	if request.DestinationGroup != "" {
		// ディレクトリは、Commit が書き込み path を解決する前に作成される。
		directory, dirErr := AbsolutePath(root, GroupDirectory(request.DestinationGroup))
		if dirErr != nil {
			return planned{}, dirErr
		}
		prepared.directories = append(prepared.directories, directory)
		prepared.preview.Notices = appendNotice(prepared.preview.Notices, Notice{
			Code:   NoticeGroupDirectoryCreated,
			Path:   GroupDirectory(request.DestinationGroup),
			Detail: request.DestinationGroup,
		})
	}

	pending := map[string][]byte{
		filepath.Clean(sourceAbsolute):      sourceUpdated,
		filepath.Clean(destinationAbsolute): destinationUpdated,
	}
	after, err := s.resolveWith(pending)
	if err != nil {
		return planned{}, err
	}
	for _, alias := range movedAliases(moved) {
		if len(prepared.preview.Effective) >= maxEffectivePreviews {
			break
		}
		prepared.preview.Effective = append(prepared.preview.Effective, DiffEffective(
			ComputeEffective(graph, root, alias, s.localFacts()),
			ComputeEffective(after, root, alias, s.localFacts()),
		))
	}
	return prepared, nil
}

func (s *Service) planMetadataEdit(graph *config.Graph, request EditRequest) (planned, error) {
	if request.Metadata == nil {
		return planned{}, ErrUnknownEditKind
	}
	root := s.workspace.Root()
	hosts, _ := ProjectHosts(graph, root)
	identities := make([]HostIdentity, 0, len(hosts))
	for _, host := range hosts {
		if !host.Identity.IsZero() {
			identities = append(identities, host.Identity)
		}
	}
	reconciled, notices := ReconcileMetadata(*request.Metadata, identities)

	_, metadataPrecondition, err := s.metadata.Load()
	if err != nil {
		return planned{}, err
	}
	metadataChange, err := s.metadata.Change(reconciled, metadataPrecondition)
	if err != nil {
		return planned{}, err
	}
	previousMetadata, _, err := s.readFile(metadataChange.Path)
	if err != nil {
		return planned{}, err
	}

	prepared := planned{
		operation: "config." + string(request.Kind),
		changes:   []storage.Change{metadataChange},
		base:      map[string][]byte{filepath.Clean(metadataChange.Path): previousMetadata},
		baseline:  diagnosticBaseline(graph),
		preview: SavePreview{
			Operation: "config." + string(request.Kind),
			Diffs: []FileDiff{BuildFileDiff(
				s.displayPath(metadataChange.Path), previousMetadata, metadataChange.Contents)},
			Notices: notices,
		},
	}
	if request.Kind == EditMetadata {
		return prepared, nil
	}

	groupsRelative := reconciled.GroupsPath()
	groupsAbsolute, err := AbsolutePath(root, groupsRelative)
	if err != nil {
		return planned{}, err
	}
	if _, err := s.workspace.ResolveForWrite(groupsAbsolute); err != nil {
		return planned{}, err
	}
	previousGroups, groupsExist, err := s.readFile(groupsAbsolute)
	if err != nil {
		return planned{}, err
	}
	entryContents, entryExists, err := s.readFile(s.entryPath)
	if err != nil {
		return planned{}, err
	}
	entryFile := config.Parse(entryContents)

	// 宣言済み集合は、生成領域が既に指定しているものすべてに、metadata
	declared := declaredGroupSet(entryFile, reconciled)
	regionPlan, err := PlanRegion(entryFile, declared, groupsRelative)
	if err != nil {
		return planned{}, err
	}
	if err := ApplyRegion(entryFile, regionPlan); err != nil {
		return planned{}, err
	}
	entryUpdated := entryFile.Render()

	pending := map[string][]byte{}
	if !bytes.Equal(entryUpdated, entryContents) {
		entryPrecondition := storage.Precondition{}
		if entryExists {
			entryPrecondition = storage.Precondition{Exists: true, Digest: storage.Digest(entryContents)}
		}
		prepared.changes = append(prepared.changes, storage.Change{
			Path: s.entryPath, Contents: entryUpdated, Precondition: entryPrecondition,
		})
		prepared.base[filepath.Clean(s.entryPath)] = entryContents
		prepared.preview.Diffs = append(prepared.preview.Diffs,
			BuildFileDiff(entryFileName, diskOrNil(entryContents, entryExists), entryUpdated))
		pending[filepath.Clean(s.entryPath)] = entryUpdated
	}

	reachable, err := s.resolveWith(pending)
	if err != nil {
		return planned{}, err
	}
	members, _ := ProjectHosts(reachable, root)
	groupContents, groupNotices := CompileGroups(declared, reconciled, members, dominantEnding(entryFile))
	prepared.preview.Notices = append(prepared.preview.Notices, groupNotices...)

	groupsPrecondition := storage.Precondition{}
	if groupsExist {
		groupsPrecondition = storage.Precondition{Exists: true, Digest: storage.Digest(previousGroups)}
	}
	prepared.changes = append(prepared.changes, storage.Change{
		Path: groupsAbsolute, Contents: groupContents, Precondition: groupsPrecondition,
	})
	prepared.base[filepath.Clean(groupsAbsolute)] = previousGroups
	prepared.preview.Diffs = append(prepared.preview.Diffs,
		BuildFileDiff(groupsRelative, diskOrNil(previousGroups, groupsExist), groupContents))
	pending[filepath.Clean(groupsAbsolute)] = groupContents
	for _, name := range declared {
		absolute, dirErr := AbsolutePath(root, GroupDirectory(name))
		if dirErr != nil {
			return planned{}, dirErr
		}
		prepared.directories = append(prepared.directories, absolute)
	}

	after, err := s.resolveWith(pending)
	if err != nil {
		return planned{}, err
	}
	afterHosts, _ := ProjectHosts(after, root)
	for _, host := range afterHosts {
		if host.Group == "" || host.Identity.IsZero() || len(prepared.preview.Effective) >= maxEffectivePreviews {
			continue
		}
		diff := DiffEffective(
			ComputeEffective(graph, root, host.Identity.Alias, s.localFacts()),
			ComputeEffective(after, root, host.Identity.Alias, s.localFacts()),
		)
		if len(diff.Changes) == 0 {
			continue
		}
		prepared.preview.Effective = append(prepared.preview.Effective, diff)
	}
	return prepared, nil
}
