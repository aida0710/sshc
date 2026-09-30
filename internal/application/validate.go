package application

import (
	"bytes"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"sshc/internal/config"
	"sshc/internal/knownhosts"
	"sshc/internal/platform/nativepath"
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
	pending := make(map[string][]byte, len(request.Changes)+len(request.FinalChanges)+len(request.Moves))
	gone := make(map[string]bool, len(request.Moves)+len(request.Removals))
	for _, changes := range [][]storage.Change{request.Changes, request.FinalChanges} {
		for _, change := range changes {
			pending[filepath.Clean(change.Path)] = change.Contents
		}
	}
	for _, move := range request.Moves {
		gone[filepath.Clean(move.From)] = true
	}
	for _, removal := range request.Removals {
		gone[filepath.Clean(removal.Path)] = true
	}
	return pending, gone
}

// configurationEdit は、このサービスが計画した設定の編集の検査の文脈で、
// storage.Request.Validation に載せて validate へ渡す。
type configurationEdit struct {
	// base は、編集したファイルの編集前の内容。すでに parse できない行を、
	// この編集が持ち込んだものと取り違えないために使う。
	base map[string][]byte
	// baseline は、編集前のグラフにすでにあった診断。
	baseline map[string]bool
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
	// A remote snapshot is an exact replica of another workspace, not a UI edit
	// authored against this installation's parser. A snapshot includes private
	// keys, certificates and OS metadata beside OpenSSH configuration; treating
	// every changed file as config makes otherwise valid snapshots impossible to
	// receive. OpenSSH also accepts directives and quoting forms which sshc may
	// preserve but not yet structure, and a receive-only replica must reproduce
	// remote breakage for inspection.
	// Rejecting those bytes here makes a snapshot upload successfully on one
	// machine and then become impossible to receive on another. Snapshot path,
	// mode, manifest and logical secret documents are validated by remotesync and
	// storage before this boundary; diagnostics can report unsupported config
	// lines after the exact bytes have been restored.
	if request.Operation == "sync.pull" || request.Operation == "sync.ignore" {
		return nil
	}
	// known_hosts は ssh_config ではない。行末のコメントの ' や " を引用として読むと、
	// 正しい known_hosts を構文の誤りとして断り、Known Hosts 画面の追加も、接続で
	// 受け入れた鍵の保存もできなくなる。書く行は knownhosts が鍵の種類と表記を
	// 確かめてから組み立てている。ここでは書く先だけを確かめる。
	if request.Operation == knownhosts.OperationAdd || request.Operation == knownhosts.OperationDelete {
		return s.checkKnownHostsDestinations(request)
	}
	pending, gone := overlayFor(request)
	edit, planned := request.Validation.(configurationEdit)

	metadataPath := filepath.Clean(s.metadata.Path())
	engineSettingsPath := filepath.Clean(s.engineSettingsPath())
	stateDir := filepath.Clean(s.workspace.StateDir())
	for _, changes := range [][]storage.Change{request.Changes, request.FinalChanges} {
		for _, change := range changes {
			cleaned := filepath.Clean(change.Path)
			if cleaned == metadataPath {
				// 読めるだけでなく、保存できる形かも確かめる。履歴から戻した文書が保存の
				// 検査を通らないと、以後の metadata の保存がすべて断られ、画面から直せない。
				decoded, err := DecodeMetadata(change.Contents)
				if err != nil {
					return err
				}
				if err := ValidateMetadata(decoded); err != nil {
					return err
				}
				continue
			}
			if cleaned == engineSettingsPath {
				if _, err := decodeEngineSettings(change.Contents); err != nil {
					return err
				}
				continue
			}
			if nativepath.Contains(stateDir, cleaned) {
				continue
			}
			parsed := config.Parse(change.Contents)
			if !bytes.Equal(parsed.Render(), change.Contents) {
				return &SyntaxError{Path: s.displayPath(cleaned), Line: 1, Column: 1, Detail: "parsed file does not render back to the same bytes"}
			}
			var base *config.File
			if contents, ok := edit.base[cleaned]; ok {
				base = config.Parse(contents)
			}
			if line, column, found := newUnstructuredLine(base, parsed); found {
				return &SyntaxError{Path: s.displayPath(cleaned), Line: line, Column: column, Detail: "unbalanced quoting"}
			}
		}
	}

	if !s.touchesConfiguration(request) {
		return nil
	}

	baseline := edit.baseline
	if !planned {
		// このサービスが計画していない要求（鍵ファイルの書き込みなど）は、ディスク上の
		// グラフにすでにある診断を基準にする。基準が無いと、~/.ssh/config が無い
		// マシンでは既存の IncludeUnreadable を新しく入ったものと扱い、設定と関係の
		// ない書き込みまで断ってしまう。validate は storage の書き込みロックの中で
		// 走るので、ここで読むディスクの状態は要求の前提と同じである。
		current, err := s.resolve()
		if err != nil {
			return err
		}
		baseline = diagnosticBaseline(current)
	}
	resolver := s.resolver
	resolver.Loader = overlayLoader{base: s.resolver.Loader, pending: pending, gone: gone}
	graph, err := resolver.Resolve(s.entryPath)
	if err != nil {
		return err
	}
	var introduced []DiagnosticView
	for _, diagnostic := range graph.Diagnostics {
		if diagnostic.Severity != config.SeverityError || baseline[diagnosticKey(diagnostic)] {
			continue
		}
		introduced = append(introduced, NewDiagnosticView(s.workspace.Root(), diagnostic))
	}
	if len(introduced) > 0 {
		return &GraphError{Diagnostics: introduced}
	}
	return nil
}

// checkKnownHostsDestinations は、known_hosts の操作が sshc の状態（metadata.json など）と
// Include グラフの設定ファイルを書かないことを確かめる。
//
// UserKnownHostsFile が誤ってそれらを指すと、接続で受け入れた鍵の行がそこに足される。
// known_hosts の操作は中身を ssh_config として検証しないので、ここで断らないと止まらない。
func (s *Service) checkKnownHostsDestinations(request storage.Request) error {
	graph, err := s.resolve()
	if err != nil {
		return err
	}
	stateDir := filepath.Clean(s.workspace.StateDir())
	for _, changes := range [][]storage.Change{request.Changes, request.FinalChanges} {
		for _, change := range changes {
			cleaned := filepath.Clean(change.Path)
			if nativepath.Contains(stateDir, cleaned) || graph.Nodes[cleaned] != nil {
				return fmt.Errorf("%w: %s is not a known_hosts file", ErrNotEditable, s.displayPath(cleaned))
			}
		}
	}
	return nil
}

func (s *Service) touchesConfiguration(request storage.Request) bool {
	stateDir := filepath.Clean(s.workspace.StateDir())
	metadataPath := filepath.Clean(s.metadata.Path())
	outside := func(path string) bool {
		cleaned := filepath.Clean(path)
		return cleaned != metadataPath && !nativepath.Contains(stateDir, cleaned)
	}
	for _, changes := range [][]storage.Change{request.Changes, request.FinalChanges} {
		for _, change := range changes {
			if outside(change.Path) {
				return true
			}
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
