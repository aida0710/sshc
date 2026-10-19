package application

import (
	"path/filepath"

	"sshc/internal/config"
)

type hostRelocation struct {
	from HostIdentity
	to   HostIdentity
}

func (s *Service) planHostEdits(graph *config.Graph, request EditRequest) (planned, error) {
	files, err := s.loadHostEditSources(request.Hosts, request.Bases)
	if err != nil {
		return planned{}, err
	}
	operation := "config." + string(request.Kind)
	prepared := planned{
		operation: operation, base: map[string][]byte{}, baseline: diagnosticBaseline(graph),
		preview: SavePreview{Operation: operation, Diffs: []FileDiff{}},
	}
	var relocations []hostRelocation
	for _, host := range request.Hosts {
		// 選択元のファイルは loadHostEditSources が bases から読み込み済みである。
		source, err := s.loadHostEditFile(files, host.Path, nil)
		if err != nil {
			return planned{}, err
		}
		if request.Kind == EditDeleteHosts {
			// 画面の removeHostBlock が HostForm.Raw と CommentLines で消す
			// 範囲は、ExtractHostBlock と同じく次のブロックのコメントを残す。
			if _, err := ExtractHostBlock(source.file, host.Alias); err != nil {
				return planned{}, err
			}
			source.touched = true
			continue
		}
		moveRequest := request
		moveRequest.Path = source.relative
		destination, err := s.hostEditDestination(graph, files, moveRequest)
		if err != nil {
			return planned{}, err
		}
		if source == destination {
			if _, found := FindHostBlock(source.file, host.Alias); !found {
				return planned{}, ErrHostNotFound
			}
			continue
		}
		if _, err := MoveHostBlock(source.file, destination.file, host.Alias); err != nil {
			return planned{}, err
		}
		source.touched, destination.touched = true, true
		relocations = append(relocations, hostRelocation{
			from: HostIdentity{Path: source.relative, Alias: host.Alias},
			to:   HostIdentity{Path: destination.relative, Alias: host.Alias},
		})
		if !s.destinationWillBeRead(graph, destination.absolute, destination.relative) {
			prepared.preview.Notices = appendNotice(prepared.preview.Notices, Notice{
				Code: NoticeDestinationNotIncluded, Path: destination.relative, Detail: host.Alias,
			})
		}
	}
	if err := files.appendChanges(&prepared); err != nil {
		return planned{}, err
	}
	if len(relocations) == 0 {
		return prepared, nil
	}
	if err := s.appendHostRelocations(&prepared, relocations); err != nil {
		return planned{}, err
	}
	if request.DestinationGroup != "" {
		directory := GroupDirectory(request.DestinationGroup)
		absolute, err := AbsolutePath(s.workspace.Root(), directory)
		if err != nil {
			return planned{}, err
		}
		prepared.directories = append(prepared.directories, absolute)
		prepared.preview.Notices = appendNotice(prepared.preview.Notices, Notice{
			Code: NoticeGroupDirectoryCreated, Path: directory, Detail: request.DestinationGroup,
		})
	}
	return prepared, nil
}

func (s *Service) hostEditDestination(graph *config.Graph, files *hostEditFiles, request EditRequest) (*hostEditFile, error) {
	resolved, err := s.resolveDestination(graph, request)
	if err != nil {
		return nil, err
	}
	base := &resolved.DestinationBase
	if resolved.DestinationGroup != "" && resolved.DestinationBase == "" {
		// 移動先も選択元なら、先に登録した画面の base を共有する。
		base = nil
	}
	return s.loadHostEditFile(files, resolved.DestinationPath, base)
}

func (s *Service) appendHostRelocations(prepared *planned, relocations []hostRelocation) error {
	metadata, precondition, err := s.metadata.Load()
	if err != nil {
		return err
	}
	for _, relocation := range relocations {
		metadata = RenameHostIdentity(metadata, relocation.from, relocation.to)
	}
	change, err := s.metadata.Change(metadata, precondition)
	if err != nil {
		return err
	}
	previous, _, err := s.readFile(change.Path)
	if err != nil {
		return err
	}
	prepared.changes = append(prepared.changes, change)
	prepared.base[filepath.Clean(change.Path)] = previous
	prepared.preview.Diffs = append(prepared.preview.Diffs,
		BuildFileDiff(s.displayPath(change.Path), previous, change.Contents))
	return nil
}
