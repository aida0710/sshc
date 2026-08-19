package application

import (
	"bytes"
	"path/filepath"

	"sshc/internal/config"
	"sshc/internal/storage"
)

// この一式は、EditRequest ひとつを planned へ落とす。
//
// **どれも書かない。** 計画を組むだけで、ディスクへ届けるのは commit の側である。
// service.go に同居していたころ、あそこは 1450 行あり、サービスの入口と計画の
// 中身が同じ画面に並んでいた。

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
		// ファイル全体の差し替えはブロックを探さない。**探す相手が居ない** ——
		// 送られてきたのは、この綴りのファイルがこれからどうあるべきかである。
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

	// コメントは同じホストの note を退役させる。両者は 2 箇所に書かれた
	// 同じものであり、このアプリケーション無しでも生き残るのは設定の
	// 方なので、note はコメントと食い違ったまま残されるのではなく、
	// コメントを書くのと同じ transaction に入る。
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
// 両方の設定ファイルと metadata 文書は 1 個の storage.Request なので、
// move は単一の journal 化された transaction である。何かが stage される前に
// すべての事前条件がチェックされ、どちらかのファイルが一致しなければ何も書き込まれない。
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
	// グループを名指した move は destination ファイルを名指していなかった
	// ので、クライアントはそれを読んだことがなく、そのバイト列を渡す
	// こともできなかった。ディスク上のファイルをその空の base と比較する
	// と、既に connection を保持しているグループファイルはすべて、アプリケーション外部で
	// 変更されたものとして報告されてしまう——そのため、グループへの最初の connection は
	// 成功し、それ以降のものはすべて、起きてもいない外部編集についてのメッセージで失敗した。
	//
	// 保証は変わっていない。下にある事前条件は依然としてたった今
	// 読んだものの digest を運び、storage は commit 中にそれを再チェック
	// するので、この読み取りと書き込みの間に変化したファイルは、
	// 依然として transaction 全体を止める。
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
		// ここまでたどり着いた plan のためにのみ作成されるので、拒否は
		// 空のディレクトリを後に残さない。
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

	// ブロックを移動すると、OpenSSH がそれを読む場所が変わり、OpenSSH は
	// 見つけた最初の値を保持する。何も変わらなかったと仮定するのでは
	// なく、そのブロックが宣言するすべての具体的な alias について、前後の説明を示す。
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

	// グループの compilation は、生成された設定ファイルと、まだ
	// include されていない場合はエントリファイル内の 1 本の Include 行も書き込む。
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

	// 宣言済み集合は、生成領域が既に名指しているものすべてに、metadata
	// が presentation を運んでいるすべてのグループを加えたものである。
	// グループを宣言することが、そもそもそのディレクトリをグループにしているものなので、
	// これで全部である。誰も宣言しなかったディレクトリは、他人のディレクトリのままである。
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

	// membership は、入ってきた設定からではなく、生成領域が生み出す設定から読まなければ
	// ならない。生成領域がグループのディレクトリを名指すまでは何もそれを読まないので、
	// この save が宣言しようとしているグループ内のすべてのホストは、
	// そうしなければ見えないままになり、その settings ブロックは空のまま出てきてしまう。
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
	// ディレクトリがまだ存在しない宣言済みグループは、
	// include_no_match 警告を生むだけで他には何も生まないので、ディレクトリは
	// 最初に到着するホストに任せるのではなく、ここで作成される。
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
	// 説明すべきホストは、今の設定が持っているものではなく、保存後の
	// 設定が持つことになるものである。グループのファイルは生成領域がそれらを名指すまで読めな
	// いので、そうしなければ、グループと共に到着するホストはここで見えないままになってしまう。
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
