package application

import (
	"bytes"
	"errors"
	"path/filepath"

	"sshc/internal/config"
	"sshc/internal/storage"
)

// MaxHostEdits は、1 トランザクションで扱う接続先数を API 契約に合わせて制限する。
const MaxHostEdits = 256

var ErrInvalidHostSelection = errors.New("hosts must contain 1-256 distinct targets with consistent file bases")

// hostEditFiles は、同じファイルへの変更を読み込み順に集約する。
type hostEditFiles struct {
	byPath map[string]*hostEditFile
	order  []*hostEditFile
}

type hostEditFile struct {
	absolute string
	relative string
	base     []byte
	disk     []byte
	exists   bool
	file     *config.File
	touched  bool
}

func (s *Service) loadHostEditFile(files *hostEditFiles, relative string, base *string) (*hostEditFile, error) {
	absolute, err := AbsolutePath(s.workspace.Root(), relative)
	if err != nil {
		return nil, err
	}
	if existing := files.byPath[absolute]; existing != nil {
		if base != nil && string(existing.base) != *base {
			return nil, ErrInvalidHostSelection
		}
		return existing, nil
	}
	if _, err := s.workspace.ResolveForWrite(absolute); err != nil {
		return nil, err
	}
	disk, exists, err := s.readConfigurationFile(absolute)
	if err != nil {
		return nil, err
	}
	contents := disk
	if base != nil {
		contents = []byte(*base)
	}
	relative, err = RelativePath(s.workspace.Root(), absolute)
	if err != nil {
		return nil, err
	}
	loaded := &hostEditFile{
		absolute: absolute, relative: relative, base: contents, disk: disk,
		exists: exists, file: config.Parse(contents),
	}
	files.byPath[absolute] = loaded
	files.order = append(files.order, loaded)
	return loaded, nil
}

func (s *Service) loadHostEditSources(hosts []HostEdit) (*hostEditFiles, error) {
	if len(hosts) == 0 || len(hosts) > MaxHostEdits {
		return nil, ErrInvalidHostSelection
	}
	files := &hostEditFiles{byPath: map[string]*hostEditFile{}}
	seen := map[HostIdentity]bool{}
	for _, host := range hosts {
		file, err := s.loadHostEditFile(files, host.Path, &host.Base)
		if err != nil {
			return nil, err
		}
		identity := HostIdentity{Path: file.relative, Alias: host.Alias}
		if host.Alias == "" || seen[identity] {
			return nil, ErrInvalidHostSelection
		}
		seen[identity] = true
	}
	return files, nil
}

func (files *hostEditFiles) appendChanges(prepared *planned) error {
	for _, file := range files.order {
		updated := file.file.Render()
		// 飛ばす接続先も画面が読んだ状態か確かめ、古い選択を黙って通さない。
		if !bytes.Equal(file.base, file.disk) {
			return &ConflictError{Report: BuildConflictReport(file.relative, file.base, file.disk, updated)}
		}
		if !file.touched {
			continue
		}
		prepared.changes = append(prepared.changes, storage.Change{
			Path: file.absolute, Contents: updated, Precondition: preconditionFor(file.base, file.exists),
		})
		prepared.base[filepath.Clean(file.absolute)] = file.base
		prepared.preview.Diffs = append(prepared.preview.Diffs,
			BuildFileDiff(file.relative, diskOrNil(file.disk, file.exists), updated))
	}
	return nil
}
