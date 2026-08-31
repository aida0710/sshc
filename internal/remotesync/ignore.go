package remotesync

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"sshc/internal/storage"
)

// IgnorePath is the workspace-relative path of the shared synchronization
// exclusion rules. The file itself always travels so every installation uses
// the same inclusion boundary.
const IgnorePath = ".sshcignore"

// MaxIgnoreBytes bounds both the editor payload and parsing work.
const MaxIgnoreBytes = 32 << 10

// DefaultIgnoreDocument is used until a workspace has saved its own rules.
// Saving the editor writes this document to .sshcignore, after which that file
// is the exact shared source of truth.
const DefaultIgnoreDocument = `# OS metadata files
**/.DS_Store
**/Thumbs.db
**/desktop.ini

# Backup and temporary files
*.bak
*.tmp

# Lock files
*.lock
`

var ErrInvalidIgnoreRules = errors.New("the synchronization exclusion rules are not valid")

type ignoreRule struct {
	negated bool
	match   *regexp.Regexp
}

// IgnoreRules is an ordered Gitignore-style rule set. Later matching lines
// override earlier lines. It deliberately operates only on workspace-relative
// slash-separated paths.
type IgnoreRules struct {
	rules []ignoreRule
}

func CompileIgnoreRules(document []byte) (IgnoreRules, error) {
	if len(document) > MaxIgnoreBytes || bytes.IndexByte(document, 0) >= 0 {
		return IgnoreRules{}, ErrInvalidIgnoreRules
	}
	lines := strings.Split(strings.ReplaceAll(string(document), "\r\n", "\n"), "\n")
	compiled := IgnoreRules{rules: make([]ignoreRule, 0, len(lines))}
	for number, raw := range lines {
		line := strings.TrimSuffix(raw, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		negated := false
		if strings.HasPrefix(line, "\\#") || strings.HasPrefix(line, "\\!") {
			line = line[1:]
		} else if strings.HasPrefix(line, "!") {
			negated = true
			line = line[1:]
		}
		if line == "" {
			return IgnoreRules{}, fmt.Errorf("%w: line %d", ErrInvalidIgnoreRules, number+1)
		}
		expression, err := ignoreExpression(line)
		if err != nil {
			return IgnoreRules{}, fmt.Errorf("%w: line %d", ErrInvalidIgnoreRules, number+1)
		}
		matcher, err := regexp.Compile(expression)
		if err != nil {
			return IgnoreRules{}, fmt.Errorf("%w: line %d", ErrInvalidIgnoreRules, number+1)
		}
		compiled.rules = append(compiled.rules, ignoreRule{
			negated: negated,
			match:   matcher,
		})
	}
	return compiled, nil
}

func ignoreExpression(pattern string) (string, error) {
	pattern = strings.TrimSuffix(pattern, "/")
	anchored := strings.HasPrefix(pattern, "/")
	pattern = strings.TrimPrefix(pattern, "/")
	if pattern == "" || strings.HasSuffix(pattern, "\\") {
		return "", ErrInvalidIgnoreRules
	}
	hasSlash := strings.Contains(pattern, "/")

	var body strings.Builder
	for index := 0; index < len(pattern); index++ {
		character := pattern[index]
		switch character {
		case '\\':
			index++
			if index >= len(pattern) {
				return "", ErrInvalidIgnoreRules
			}
			body.WriteString(regexp.QuoteMeta(string(pattern[index])))
		case '*':
			if index+1 < len(pattern) && pattern[index+1] == '*' {
				index++
				if index+1 < len(pattern) && pattern[index+1] == '/' {
					index++
					body.WriteString("(?:.*/)?")
				} else {
					body.WriteString(".*")
				}
			} else {
				body.WriteString("[^/]*")
			}
		case '?':
			body.WriteString("[^/]")
		case '[':
			end := index + 1
			if end < len(pattern) && (pattern[end] == '!' || pattern[end] == '^') {
				end++
			}
			if end < len(pattern) && pattern[end] == ']' {
				end++
			}
			for end < len(pattern) && pattern[end] != ']' {
				end++
			}
			if end >= len(pattern) {
				return "", ErrInvalidIgnoreRules
			}
			class := pattern[index+1 : end]
			if strings.HasPrefix(class, "!") {
				class = "^" + class[1:]
			}
			body.WriteByte('[')
			body.WriteString(class)
			body.WriteByte(']')
			index = end
		default:
			body.WriteString(regexp.QuoteMeta(string(character)))
		}
	}

	prefix := "^"
	if !anchored && !hasSlash {
		prefix = "(?:^|.*/)"
	}
	// Gitignore patterns also match a directory bearing that name, which makes
	// every file below it ignored. Evaluating only file paths means representing
	// that behavior as an optional descendant suffix.
	suffix := "(?:/.*)?$"
	return prefix + body.String() + suffix, nil
}

// Match reports whether relative is excluded after applying every rule. The
// control file and the two logical encrypted documents cannot exclude
// themselves, even through a broad pattern such as *. The logical documents
// are not ordinary workspace files and have their own validated transfer path.
func (rules IgnoreRules) Match(relative string) bool {
	relative = strings.TrimPrefix(strings.ReplaceAll(relative, "\\", "/"), "./")
	if relative == "" || relative == IgnorePath || relative == TravelPath || relative == SnippetsPath {
		return false
	}
	ignored := false
	for _, rule := range rules.rules {
		if rule.match.MatchString(relative) {
			ignored = !rule.negated
		}
	}
	return ignored
}

func ignoreRulesForDocument(document []byte, present bool) (IgnoreRules, []byte, error) {
	if !present {
		document = []byte(DefaultIgnoreDocument)
	}
	rules, err := CompileIgnoreRules(document)
	return rules, document, err
}

type ExclusionCandidate struct {
	Path    string
	Ignored bool
}

type ExclusionView struct {
	Document      string
	UsingDefaults bool
	Candidates    []ExclusionCandidate
}

func (s *Service) loadIgnoreRules() (IgnoreRules, []byte, bool, error) {
	absolute := filepath.Join(s.workspace.Root(), IgnorePath)
	document, err := s.workspace.FileSystem().ReadFile(absolute)
	present := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return IgnoreRules{}, nil, false, err
	}
	rules, effective, err := ignoreRulesForDocument(document, present)
	return rules, effective, present, err
}

func ignoreRulesFromSnapshot(contents map[string][]byte) (IgnoreRules, error) {
	document, present := contents[IgnorePath]
	rules, _, err := ignoreRulesForDocument(document, present)
	return rules, err
}

// Exclusions returns the effective shared document and current regular-file
// candidates without reading their contents.
func (s *Service) Exclusions() (ExclusionView, error) {
	rules, document, present, err := s.loadIgnoreRules()
	if err != nil {
		return ExclusionView{}, err
	}
	relatives, err := s.walkWorkspace()
	if err != nil {
		return ExclusionView{}, err
	}
	candidates := make([]ExclusionCandidate, 0, len(relatives))
	for _, relative := range relatives {
		relative = filepath.ToSlash(relative)
		if relative == IgnorePath {
			continue
		}
		candidates = append(candidates, ExclusionCandidate{Path: relative, Ignored: rules.Match(relative)})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Path < candidates[j].Path })
	return ExclusionView{Document: string(document), UsingDefaults: !present, Candidates: candidates}, nil
}

// SaveExclusions validates and atomically saves the exact shared document.
func (s *Service) SaveExclusions(document string) (ExclusionView, error) {
	if _, err := CompileIgnoreRules([]byte(document)); err != nil {
		return ExclusionView{}, err
	}
	absolute := filepath.Join(s.workspace.Root(), IgnorePath)
	precondition := storage.Precondition{}
	current, err := s.workspace.FileSystem().ReadFile(absolute)
	if err == nil {
		if string(current) == document {
			return s.Exclusions()
		}
		precondition = storage.Precondition{Exists: true, Digest: storage.Digest(current)}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return ExclusionView{}, err
	}
	if _, err := s.transactions.Commit(storage.Request{
		Operation: "sync.ignore",
		Changes:   []storage.Change{{Path: absolute, Contents: []byte(document), Precondition: precondition}},
	}); err != nil {
		return ExclusionView{}, err
	}
	return s.Exclusions()
}
