package application

import (
	"errors"
	"strings"

	"sshc/internal/config"
	"sshc/internal/effective"
)

var ErrHostNotFound = errors.New("host block not found")

// FieldCategory は、ホスト editor のどの tab がディレクティブを示すかを決める。
type FieldCategory string

const (
	CategoryBasic    FieldCategory = "basic"
	CategoryJump     FieldCategory = "jump"
	CategoryAdvanced FieldCategory = "advanced"
)

// basicKeywords と jumpKeywords は、専用の form を持つディレクティブを保持する。
var basicKeywords = map[string]bool{
	"hostname": true, "user": true, "port": true, "identityfile": true,
	"identitiesonly": true, "addkeystoagent": true, "tag": true,
}

var jumpKeywords = map[string]bool{
	"proxyjump": true, "proxycommand": true, "forwardagent": true, "requesttty": true,
}

var dangerousKeywords = map[string]bool{
	"proxycommand": true, "knownhostscommand": true, "localcommand": true,
	"remotecommand": true, "permitlocalcommand": true,
}

// CategoryFor は、ディレクティブがどこに属するかを決める。
func CategoryFor(keyword string) FieldCategory {
	lowered := strings.ToLower(keyword)
	switch {
	case basicKeywords[lowered]:
		return CategoryBasic
	case jumpKeywords[lowered]:
		return CategoryJump
	default:
		return CategoryAdvanced
	}
}

// IsDangerousKeyword は、値を OpenSSH が実行できるディレクティブを報告する。
func IsDangerousKeyword(keyword string) bool {
	return dangerousKeywords[strings.ToLower(keyword)]
}

type FormField struct {
	Line      int           `json:"line"`
	Keyword   string        `json:"keyword"`
	Values    []string      `json:"values"`
	Category  FieldCategory `json:"category"`
	Dangerous bool          `json:"dangerous,omitempty"`
	Duplicate bool          `json:"duplicate,omitempty"`
	Editable  bool          `json:"editable"`
}

type FileRef struct {
	Path     string `json:"path,omitempty"`
	Absolute string `json:"absolute"`
	External bool   `json:"external,omitempty"`
}

func NewFileRef(root, absolute string) FileRef {
	relative, err := RelativePath(root, absolute)
	if err != nil {
		return FileRef{Absolute: absolute, External: true}
	}
	return FileRef{Path: relative, Absolute: absolute}
}

// HostEntry は、tree が示すとおりの 1 個の Host ブロックである。
type HostEntry struct {
	Identity  HostIdentity `json:"identity"`
	File      FileRef      `json:"file"`
	Line      int          `json:"line"`
	Patterns  []string     `json:"patterns"`
	Wildcard  bool         `json:"wildcard,omitempty"`
	Negated   bool         `json:"negated,omitempty"`
	Duplicate bool         `json:"duplicate,omitempty"`
	Editable  bool         `json:"editable"`
	Group     string       `json:"group,omitempty"`
	HostName  string       `json:"hostName,omitempty"`
	User      string       `json:"user,omitempty"`
	Port      string       `json:"port,omitempty"`
}

// HostForm は、detail editor のために射影された 1 個の Host
type HostForm struct {
	Entry        HostEntry   `json:"entry"`
	Fields       []FormField `json:"fields"`
	Raw          string      `json:"raw"`
	Comment      string      `json:"comment"`
	CommentLines int         `json:"commentLines"`
	Notices      []Notice    `json:"notices,omitempty"`
}

func PrimaryAlias(patterns []config.Pattern) string {
	for _, pattern := range patterns {
		if pattern.Negated || pattern.Wildcard {
			continue
		}
		return pattern.Value
	}
	return ""
}

// MatchHostLine は、match する negated pattern が 1 個でもあれば行全体を却下し、positive
func MatchHostLine(patterns []config.Pattern, candidate string) bool {
	matched := false
	for _, pattern := range patterns {
		if !effective.MatchPattern(pattern.Value, candidate) {
			continue
		}
		if pattern.Negated {
			return false
		}
		matched = true
	}
	return matched
}

func ProjectHosts(graph *config.Graph, root string) ([]HostEntry, []Notice) {
	var hosts []HostEntry
	var notices []Notice
	seen := map[string]bool{}

	WalkDirectives(graph, func(visit Visit) bool {
		if visit.Block.Kind != config.BlockHost || visit.Block.Header != visit.Index {
			return true
		}
		entry := HostEntry{
			File:     NewFileRef(root, visit.Path),
			Line:     visit.Index + 1,
			Patterns: make([]string, 0, len(visit.Block.Patterns)),
		}
		entry.Group, _ = GroupOfPath(entry.File.Path)
		node, ok := graph.Nodes[visit.Path]
		entry.Editable = ok && node.Editable && !entry.File.External
		for _, pattern := range visit.Block.Patterns {
			entry.Patterns = append(entry.Patterns, pattern.Raw)
			entry.Wildcard = entry.Wildcard || pattern.Wildcard
			entry.Negated = entry.Negated || pattern.Negated
		}
		if entry.File.External {
			notices = appendNotice(notices, Notice{
				Code: NoticeExternalFile, Line: entry.Line, Detail: visit.Path,
			})
		}

		if entry.Negated {
			notices = appendNotice(notices, Notice{
				Code: NoticeNegatedPattern, Path: entry.File.Path, Line: entry.Line,
				Detail: visit.Condition,
			})
			notices = appendNotice(notices, Notice{
				Code: NoticeComplexExternalRule, Path: entry.File.Path, Line: entry.Line,
				Detail: visit.Condition,
			})
		}

		alias := PrimaryAlias(visit.Block.Patterns)
		if alias == "" {
			notices = appendNotice(notices, Notice{
				Code: NoticeUnnamedHostBlock, Path: entry.File.Path, Line: entry.Line,
				Detail: visit.Condition,
			})
			notices = appendNotice(notices, Notice{
				Code: NoticeWildcardShadow, Path: entry.File.Path, Line: entry.Line,
				Detail: visit.Condition,
			})
			hosts = append(hosts, entry)
			return true
		}
		if !entry.File.External {
			entry.Identity = HostIdentity{Path: entry.File.Path, Alias: alias}
		}
		// alias だけを key にし、それが置かれているファイルは key にしない。
		if seen[alias] {
			entry.Duplicate = true
			notices = appendNotice(notices, Notice{
				Code: NoticeDuplicateAlias, Path: entry.File.Path, Line: entry.Line, Detail: alias,
			})
			notices = appendNotice(notices, Notice{
				Code: NoticeComplexExternalRule, Path: entry.File.Path, Line: entry.Line, Detail: alias,
			})
		}
		seen[alias] = true
		hosts = append(hosts, entry)
		return true
	})
	return hosts, notices
}

// ProjectHostForm は、1 個のホストブロックの詳細 view を構築する。
func ProjectHostForm(graph *config.Graph, root string, identity HostIdentity) (HostForm, error) {
	absolute, err := AbsolutePath(root, identity.Path)
	if err != nil {
		return HostForm{}, err
	}
	node, ok := graph.Nodes[absolute]
	if !ok || node.File == nil {
		return HostForm{}, ErrHostNotFound
	}
	block, ok := FindHostBlock(node.File, identity.Alias)
	if !ok {
		return HostForm{}, ErrHostNotFound
	}

	form := HostForm{
		Entry: HostEntry{
			Identity: identity,
			File:     NewFileRef(root, absolute),
			Line:     block.Header + 1,
			Patterns: make([]string, 0, len(block.Patterns)),
			Editable: node.Editable,
		},
		Fields:       []FormField{},
		Comment:      node.File.CommentText(block.Header),
		CommentLines: block.Header - node.File.CommentRun(block.Header),
	}
	form.Entry.Group, _ = GroupOfPath(form.Entry.File.Path)
	for _, pattern := range block.Patterns {
		form.Entry.Patterns = append(form.Entry.Patterns, pattern.Raw)
		form.Entry.Wildcard = form.Entry.Wildcard || pattern.Wildcard
		form.Entry.Negated = form.Entry.Negated || pattern.Negated
	}

	keywordSeen := map[string]bool{}
	var raw strings.Builder
	raw.WriteString(node.File.Lines[block.Header].Render())
	end := node.File.CommentRun(block.End)
	_, end = ClampToRegion(node.File, block.Header, end)
	for index := block.Start; index < end; index++ {
		line := node.File.Lines[index]
		raw.WriteString(line.Render())
		switch line.Kind {
		case config.LineUnstructured:
			form.Notices = appendNotice(form.Notices, Notice{
				Code: NoticeUnstructuredLine, Path: identity.Path, Line: index + 1,
			})
		case config.LineDirective:
			lowered := strings.ToLower(line.Keyword)
			field := FormField{
				Line:      index + 1,
				Keyword:   line.Keyword,
				Values:    line.Values(),
				Category:  CategoryFor(line.Keyword),
				Dangerous: IsDangerousKeyword(line.Keyword),
				Duplicate: keywordSeen[lowered],
				Editable:  node.Editable,
			}
			keywordSeen[lowered] = true
			if field.Dangerous {
				form.Notices = appendNotice(form.Notices, Notice{
					Code: NoticeDangerousDirective, Path: identity.Path, Line: field.Line, Detail: line.Keyword,
				})
			}
			form.Fields = append(form.Fields, field)
		}
	}
	form.Raw = raw.String()
	return form, nil
}

// FindHostBlock は、primary alias が match する最初の Host ブロックを返す。
func FindHostBlock(file *config.File, alias string) (config.Block, bool) {
	for _, block := range file.Blocks() {
		if block.Kind != config.BlockHost {
			continue
		}
		if PrimaryAlias(block.Patterns) == alias {
			return block, true
		}
	}
	return config.Block{}, false
}

// directIdentityFile は、一つの具体的な Host ブロック自身が指定する秘密鍵を返す。
func directIdentityFile(file *config.File, block config.Block) (Notice, bool) {
	for index := block.Start; index < block.End && index < len(file.Lines); index++ {
		line := file.Lines[index]
		if line.Kind != config.LineDirective || !strings.EqualFold(line.Keyword, "IdentityFile") {
			continue
		}
		for _, value := range line.Values() {
			value = strings.TrimSpace(value)
			if value == "" || strings.EqualFold(value, "none") {
				continue
			}
			return Notice{Code: BlockerIdentityFileConfigured, Line: index + 1, Detail: value}, true
		}
	}
	return Notice{}, false
}

func directIdentityFileForAlias(graph *config.Graph, alias string) (Notice, bool) {
	var notice Notice
	found := false
	WalkDirectives(graph, func(visit Visit) bool {
		if visit.Block.Kind != config.BlockHost || visit.Block.Header != visit.Index ||
			PrimaryAlias(visit.Block.Patterns) != alias {
			return true
		}
		found = true
		node := graph.Nodes[visit.Path]
		if node.File != nil {
			notice, _ = directIdentityFile(node.File, visit.Block)
			notice.Path = visit.Path
		}
		return false
	})
	return notice, found && notice.Code != ""
}

// DiagnosticView は、HTTP contract 向けに準備された config.Diagnostic である。
type DiagnosticView struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Path     string `json:"path,omitempty"`
	Absolute string `json:"absolute,omitempty"`
	External bool   `json:"external,omitempty"`
	Line     int    `json:"line,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

// SeverityName は、severity を contract が使う安定した文字列として表す。
func SeverityName(severity config.Severity) string {
	switch severity {
	case config.SeverityError:
		return "error"
	case config.SeverityWarning:
		return "warning"
	default:
		return "info"
	}
}

func NewDiagnosticView(root string, diagnostic config.Diagnostic) DiagnosticView {
	reference := NewFileRef(root, diagnostic.Path)
	return DiagnosticView{
		Severity: SeverityName(diagnostic.Severity),
		Code:     diagnostic.Code,
		Path:     reference.Path,
		Absolute: reference.Absolute,
		External: reference.External,
		Line:     diagnostic.Line,
		Detail:   diagnostic.Detail,
	}
}
