package application

import (
	"os"
	"os/user"
	"sort"
	"strings"

	"sshc/internal/config"
	"sshc/internal/effective"
	"sshc/internal/platform"
)

// Source は値がどこから来たかを表す。
type Source struct {
	Path      string `json:"path,omitempty"`
	Absolute  string `json:"absolute,omitempty"`
	Line      int    `json:"line,omitempty"`
	Condition string `json:"condition,omitempty"`
}

// EffectiveEntry は説明付きの 1 つの値である。
type EffectiveEntry struct {
	Keyword string   `json:"keyword"`
	Values  []string `json:"values"`
	Source  Source   `json:"source"`
}

// Effective は、alias が受け取る値である。
type Effective struct {
	Alias   string           `json:"alias"`
	Entries []EffectiveEntry `json:"entries"`
	Notices []Notice         `json:"notices,omitempty"`
}

func declaresExactly(patterns []config.Pattern, alias string) bool {
	for _, pattern := range patterns {
		if pattern.Negated || pattern.Wildcard {
			continue
		}
		if pattern.Value == alias {
			return true
		}
	}
	return false
}

// ComputeEffective は、この alias が受け取る値を返す。
func ComputeEffective(graph *config.Graph, root, alias string, facts effective.LocalFacts) Effective {
	computed := Effective{Alias: alias, Entries: []EffectiveEntry{}}
	resolution := effective.Resolve(graph, alias, facts)

	for _, refusal := range resolution.Refusals {
		computed.Notices = appendNotice(computed.Notices, Notice{
			Code: refusalNotices[refusal.Code], Path: refusal.Path,
			Line: refusal.Line, Detail: refusal.Detail,
		})
	}
	if len(resolution.Refusals) > 0 {
		return computed
	}

	// 結果は確定しているが、書いたユーザー本人には見えていないこと。
	for _, note := range resolution.Notes {
		computed.Notices = appendNotice(computed.Notices, Notice{
			Code: noteNotices[note.Code], Path: note.Path, Line: note.Line, Detail: note.Detail,
		})
	}

	for _, entry := range resolution.Accepted {
		reference := NewFileRef(root, entry.Path)
		computed.Entries = append(computed.Entries, EffectiveEntry{
			Keyword: entry.Keyword,
			Values:  entry.Values,
			Source: Source{
				Path:      reference.Path,
				Absolute:  reference.Absolute,
				Line:      entry.Line,
				Condition: entry.Condition,
			},
		})
	}
	sort.SliceStable(computed.Entries, func(first, second int) bool {
		return strings.ToLower(computed.Entries[first].Keyword) < strings.ToLower(computed.Entries[second].Keyword)
	})
	return computed
}

// LocalFactsFor は、トークン展開に要るこのプロセスの事実を読む。
func LocalFactsFor(home string) effective.LocalFacts {
	facts := effective.LocalFacts{Home: home}
	if current, err := user.Current(); err == nil {
		facts.User = platform.LocalAccountName(current.Username)
		facts.UID = current.Uid
	}
	if hostname, err := os.Hostname(); err == nil {
		facts.Hostname = hostname
	}
	return facts
}

// noteNotices は、確定した結果に添える印を画面の用語へ移す。
var noteNotices = map[string]string{
	effective.ComplexityDuplicateAlias:    NoticeDuplicateAlias,
	effective.ComplexityWildcardPattern:   NoticeWildcardShadow,
	effective.ComplexityNegatedPattern:    NoticeNegatedPattern,
	effective.ComplexityUnresolvedInclude: NoticeExplainedValuesOnly,
}

// refusalNotices は設定を解決できなかった理由を UI 用 notice へ変換する。
var refusalNotices = map[string]string{
	effective.RefusalMatchExec:    NoticeMatchExecRefused,
	effective.RefusalMatchFinal:   NoticeMatchFinalRefused,
	effective.RefusalMatchUnknown: NoticeMatchExecRefused,
	effective.RefusalCanonicalize: NoticeCanonicaliseRefused,
	effective.RefusalUnknownToken: NoticeUnknownTokenRefused,
}

// EffectiveChange は、説明付きの値が変化する 1 つの keyword である。
type EffectiveChange struct {
	Keyword       string   `json:"keyword"`
	Before        []string `json:"before"`
	After         []string `json:"after"`
	BeforeSources []Source `json:"beforeSources,omitempty"`
	AfterSources  []Source `json:"afterSources,omitempty"`
}

type EffectiveDiff struct {
	Alias   string            `json:"alias"`
	Changes []EffectiveChange `json:"changes"`
}

// DiffEffective は 2 つの説明を keyword ごとに比較する。
func DiffEffective(before, after Effective) EffectiveDiff {
	diff := EffectiveDiff{Alias: after.Alias, Changes: []EffectiveChange{}}
	if diff.Alias == "" {
		diff.Alias = before.Alias
	}
	beforeIndex := indexEffective(before)
	afterIndex := indexEffective(after)

	keywords := make([]string, 0, len(beforeIndex)+len(afterIndex))
	for keyword := range beforeIndex {
		keywords = append(keywords, keyword)
	}
	for keyword := range afterIndex {
		if _, ok := beforeIndex[keyword]; !ok {
			keywords = append(keywords, keyword)
		}
	}
	sort.Strings(keywords)

	for _, keyword := range keywords {
		beforeValues, beforeSources, display := renderEffective(beforeIndex[keyword])
		afterValues, afterSources, afterDisplay := renderEffective(afterIndex[keyword])
		if afterDisplay != "" {
			display = afterDisplay
		}
		if equalStrings(beforeValues, afterValues) && equalSources(beforeSources, afterSources) {
			continue
		}
		diff.Changes = append(diff.Changes, EffectiveChange{
			Keyword:       display,
			Before:        beforeValues,
			After:         afterValues,
			BeforeSources: beforeSources,
			AfterSources:  afterSources,
		})
	}
	return diff
}

func indexEffective(effective Effective) map[string][]EffectiveEntry {
	index := make(map[string][]EffectiveEntry, len(effective.Entries))
	for _, entry := range effective.Entries {
		lowered := strings.ToLower(entry.Keyword)
		index[lowered] = append(index[lowered], entry)
	}
	return index
}

func renderEffective(entries []EffectiveEntry) (values []string, sources []Source, display string) {
	values = []string{}
	sources = []Source{}
	for _, entry := range entries {
		if display == "" {
			display = entry.Keyword
		}
		values = append(values, strings.Join(entry.Values, " "))
		sources = append(sources, entry.Source)
	}
	return values, sources, display
}

func equalStrings(first, second []string) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range first {
		if first[index] != second[index] {
			return false
		}
	}
	return true
}

func equalSources(first, second []Source) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range first {
		if !equivalentSource(first[index], second[index]) {
			return false
		}
	}
	return true
}

func equivalentSource(first, second Source) bool {
	return first.Path == second.Path &&
		first.Absolute == second.Absolute &&
		first.Condition == second.Condition
}

// ErrUnresolvable は、その alias に接続できない理由を運ぶ。
type ErrUnresolvable struct {
	Alias   string
	Codes   []string
	Details []string
}

func (e *ErrUnresolvable) Error() string {
	return e.Alias + ": " + strings.Join(e.Details, "; ")
}

// ResolveConnection は、この alias に接続したときに実際に使われる値を返す。
func (s *Service) ResolveConnection(alias string) (effective.Values, error) {
	graph, err := s.resolve()
	if err != nil {
		return effective.Values{}, err
	}
	resolution := effective.Resolve(graph, alias, s.localFacts())
	if len(resolution.Refusals) > 0 {
		failure := &ErrUnresolvable{Alias: alias}
		for _, refusal := range resolution.Refusals {
			failure.Codes = append(failure.Codes, refusal.Code)
			failure.Details = append(failure.Details, refusal.Detail)
		}
		return effective.Values{}, failure
	}
	return resolution.Values, nil
}
