package terminalworkspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"sshc/internal/storage"
)

const temporaryName = ".workspaces-"

type document struct {
	SchemaVersion int         `json:"schemaVersion"`
	Workspaces    []Workspace `json:"workspaces"`
}

// store は workspaces.json を読み書きする。直列化は Service が持つ。
type store struct {
	workspace *storage.Workspace
}

func (s store) path() string {
	return filepath.Join(s.workspace.Root(), filepath.FromSlash(PathRelative))
}

// load は、保存したワークスペースを、保存した順（画面に並べる順）で返す。ファイルが
// 無いときと、分割の頃の形のときは、消せないワークスペースだけを返す。
func (s store) load() ([]Workspace, error) {
	contents, err := s.workspace.FileSystem().ReadFile(s.path())
	if errors.Is(err, fs.ErrNotExist) {
		return builtIns(), nil
	}
	if err != nil {
		return nil, err
	}
	var version struct {
		SchemaVersion int `json:"schemaVersion"`
	}
	if err := json.Unmarshal(contents, &version); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidDocument, err)
	}
	switch {
	case version.SchemaVersion > SchemaVersion:
		return nil, ErrUnsupportedVersion
	case version.SchemaVersion < SchemaVersion:
		return builtIns(), nil
	}
	var stored document
	if err := json.Unmarshal(contents, &stored); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidDocument, err)
	}
	return validDocument(stored.Workspaces)
}

// validDocument は、読んだ定義を確かめ、消せないワークスペースが欠けていれば足す。
func validDocument(workspaces []Workspace) ([]Workspace, error) {
	if len(workspaces) > MaxWorkspaces {
		return nil, ErrInvalidDocument
	}
	seen := make(map[string]bool, len(workspaces))
	for _, workspace := range workspaces {
		name, err := normalizeName(workspace.Name, workspace.BuiltIn())
		if workspace.ID == "" || seen[workspace.ID] || err != nil || name != workspace.Name {
			return nil, ErrInvalidDocument
		}
		seen[workspace.ID] = true
	}
	for _, builtIn := range builtIns() {
		if !seen[builtIn.ID] {
			workspaces = append(workspaces, builtIn)
		}
	}
	return workspaces, nil
}

func (s store) write(workspaces []Workspace) error {
	if err := s.workspace.EnsureDirectory(s.workspace.StateDir()); err != nil {
		return err
	}
	contents, err := json.MarshalIndent(document{SchemaVersion: SchemaVersion, Workspaces: workspaces}, "", "  ")
	if err != nil {
		return err
	}
	contents = append(contents, '\n')
	return storage.WriteAtomicFile(s.workspace.FileSystem(), s.path(), temporaryName, storage.FilePermission, contents)
}
