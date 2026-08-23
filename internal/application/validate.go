package application

import (
	"bytes"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"sshc/internal/config"
	"sshc/internal/storage"
)

type SyntaxError struct {
	Path   string
	Line   int
	Column int
	Detail string
}

func (e *SyntaxError) Error() string {
	return "configuration syntax error at line " + strconv.Itoa(e.Line)
}

// GraphError は、新しい Include graph のエラーを持ち込む save を拒否する。
type GraphError struct {
	Diagnostics []DiagnosticView
}

func (e *GraphError) Error() string { return "include graph error" }

// ConflictError は、ディスク上のファイルが編集時のものと違うことを報告する。
type ConflictError struct {
	Report ConflictReport
}

func (e *ConflictError) Error() string { return "external change detected" }

type overlayLoader struct {
	base    config.Loader
	pending map[string][]byte
	gone    map[string]bool
}

func (loader overlayLoader) ReadFile(name string) ([]byte, error) {
	cleaned := filepath.Clean(name)
	if contents, ok := loader.pending[cleaned]; ok {
		return contents, nil
	}
	if loader.gone[cleaned] {
		return nil, fs.ErrNotExist
	}
	return loader.base.ReadFile(name)
}

func (loader overlayLoader) Glob(pattern string) ([]string, error) {
	found, err := loader.base.Glob(pattern)
	if err != nil {
		return nil, err
	}
	matches := make([]string, 0, len(found))
	seen := make(map[string]bool, len(found))
	for _, match := range found {
		cleaned := filepath.Clean(match)
		if loader.gone[cleaned] && loader.pending[cleaned] == nil {
			continue
		}
		matches = append(matches, match)
		seen[cleaned] = true
	}
	for name := range loader.pending {
		if seen[name] {
			continue
		}
		matched, matchErr := filepath.Match(pattern, name)
		if matchErr != nil {
			return nil, matchErr
		}
		if matched {
			matches = append(matches, name)
		}
	}
	sort.Strings(matches)
	return matches, nil
}

func overlayFor(request storage.Request) (map[string][]byte, map[string]bool) {
	pending := make(map[string][]byte, len(request.Changes)+len(request.Moves))
	gone := make(map[string]bool, len(request.Moves)+len(request.Removals))
	for _, change := range request.Changes {
		pending[filepath.Clean(change.Path)] = change.Contents
	}
	for _, move := range request.Moves {
		gone[filepath.Clean(move.From)] = true
	}
	for _, removal := range request.Removals {
		gone[filepath.Clean(removal.Path)] = true
	}
	return pending, gone
}

func diagnosticKey(diagnostic config.Diagnostic) string {
	return diagnostic.Code + "\x00" + diagnostic.Path + "\x00" + strconv.Itoa(diagnostic.Line)
}

func diagnosticBaseline(graph *config.Graph) map[string]bool {
	baseline := make(map[string]bool, len(graph.Diagnostics))
	for _, diagnostic := range graph.Diagnostics {
		baseline[diagnosticKey(diagnostic)] = true
	}
	return baseline
}

// newUnstructuredLine は、編集が parse 不能にしてしまった行を見つける。
func newUnstructuredLine(before, after *config.File) (line, column int, found bool) {
	known := map[string]int{}
	if before != nil {
		for _, existing := range before.Lines {
			if existing.Kind == config.LineUnstructured {
				known[existing.Text]++
			}
		}
	}
	for index, candidate := range after.Lines {
		if candidate.Kind != config.LineUnstructured {
			continue
		}
		if known[candidate.Text] > 0 {
			known[candidate.Text]--
			continue
		}
		return index + 1, unstructuredColumn(candidate.Text), true
	}
	return 0, 0, false
}

func unstructuredColumn(text string) int {
	if index := strings.IndexByte(text, '"'); index >= 0 {
		return index + 1
	}
	return 1
}

func (s *Service) validate(request storage.Request) error {
	pending, gone := overlayFor(request)

	metadataPath := filepath.Clean(s.metadata.Path())
	stateDir := filepath.Clean(s.workspace.StateDir())
	for _, change := range request.Changes {
		cleaned := filepath.Clean(change.Path)
		if cleaned == metadataPath {
			if _, err := DecodeMetadata(change.Contents); err != nil {
				return err
			}
			continue
		}
		if isInside(stateDir, cleaned) {
			continue
		}
		parsed := config.Parse(change.Contents)
		if !bytes.Equal(parsed.Render(), change.Contents) {
			return &SyntaxError{Path: s.displayPath(cleaned), Line: 1, Column: 1, Detail: "parsed file does not render back to the same bytes"}
		}
		var base *config.File
		if contents, ok := s.pendingBase[cleaned]; ok {
			base = config.Parse(contents)
		}
		if line, column, found := newUnstructuredLine(base, parsed); found {
			return &SyntaxError{Path: s.displayPath(cleaned), Line: line, Column: column, Detail: "unbalanced quoting"}
		}
	}

	if !s.touchesConfiguration(request) {
		return nil
	}

	resolver := s.resolver
	resolver.Loader = overlayLoader{base: s.resolver.Loader, pending: pending, gone: gone}
	graph, err := resolver.Resolve(s.entryPath)
	if err != nil {
		return err
	}
	var introduced []DiagnosticView
	for _, diagnostic := range graph.Diagnostics {
		if diagnostic.Severity != config.SeverityError || s.pendingBaseline[diagnosticKey(diagnostic)] {
			continue
		}
		introduced = append(introduced, NewDiagnosticView(s.workspace.Root(), diagnostic))
	}
	if len(introduced) > 0 {
		return &GraphError{Diagnostics: introduced}
	}
	return nil
}

func (s *Service) touchesConfiguration(request storage.Request) bool {
	stateDir := filepath.Clean(s.workspace.StateDir())
	metadataPath := filepath.Clean(s.metadata.Path())
	outside := func(path string) bool {
		cleaned := filepath.Clean(path)
		return cleaned != metadataPath && !isInside(stateDir, cleaned)
	}
	for _, change := range request.Changes {
		if outside(change.Path) {
			return true
		}
	}
	for _, move := range request.Moves {
		if outside(move.From) || outside(move.To) {
			return true
		}
	}
	for _, removal := range request.Removals {
		if outside(removal.Path) {
			return true
		}
	}
	return len(request.Directories) > 0 || len(request.RemoveDirectories) > 0
}

// isInside は、path が directory それ自体かその下にあるかを報告する。
func isInside(directory, path string) bool {
	if path == directory {
		return true
	}
	relative, err := filepath.Rel(directory, path)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
