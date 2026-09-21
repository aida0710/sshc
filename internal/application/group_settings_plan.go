package application

import (
	"bytes"
	"path/filepath"

	"sshc/internal/config"
)

// plannedGraph includes moved files as well as writes. A glob must see the
// destination and stop seeing the source before group membership is compiled.
func (s *Service) plannedGraph(prepared planned) (*config.Graph, error) {
	pending, gone := overlayFor(s.requestFor(prepared))
	for _, move := range prepared.moves {
		contents, _, err := s.readFile(move.From)
		if err != nil {
			return nil, err
		}
		pending[filepath.Clean(move.To)] = contents
	}
	return s.resolveOverlay(pending, gone)
}

func (s *Service) plannedMetadata(prepared planned) (Metadata, error) {
	for _, change := range prepared.changes {
		if filepath.Clean(change.Path) == filepath.Clean(s.metadata.Path()) {
			return DecodeMetadata(change.Contents)
		}
	}
	metadata, _, err := s.metadata.Load()
	return metadata, err
}

// refreshGroupSettings keeps generated Host lists in the same transaction as
// membership changes. It also returns the graph used for preview and binding.
func (s *Service) refreshGroupSettings(prepared *planned, metadata Metadata) (*config.Graph, error) {
	graph, err := s.plannedGraph(*prepared)
	if err != nil {
		return nil, err
	}
	entry := graph.Nodes[s.entryPath]
	if entry == nil || entry.File == nil {
		return graph, nil
	}
	_, _, generated, err := FindRegion(entry.File)
	if err != nil || !generated {
		return graph, err
	}
	groupsPath, err := AbsolutePath(s.workspace.Root(), metadata.GroupsPath())
	if err != nil {
		return nil, err
	}
	if _, err := s.workspace.ResolveForWrite(groupsPath); err != nil {
		return nil, err
	}
	previous, exists, err := s.readFile(groupsPath)
	if err != nil {
		return nil, err
	}
	if !exists && prepared.operation != "config.groups" && !hasGroupSettings(metadata) {
		return graph, nil
	}
	hosts, _ := ProjectHosts(graph, s.workspace.Root())
	contents, notices := CompileGroups(DeclaredGroups(entry.File), metadata, hosts, dominantEnding(entry.File))
	prepared.preview.Notices = append(prepared.preview.Notices, notices...)
	if bytes.Equal(previous, contents) {
		return graph, nil
	}
	if err := s.appendRewrites(prepared, []rewrite{{
		absolute: groupsPath, display: metadata.GroupsPath(), previous: previous, updated: contents, created: !exists,
	}}); err != nil {
		return nil, err
	}
	return s.plannedGraph(*prepared)
}

func hasGroupSettings(metadata Metadata) bool {
	for _, group := range metadata.Groups {
		if len(group.Settings) > 0 {
			return true
		}
	}
	return false
}
