package sftp

import (
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// Keep matching work and persisted settings bounded even for very large trees.
	MaxTransferExclusionPatterns     = 64
	MaxTransferExclusionPatternBytes = 512
)

// ValidateTransferExclusionPatterns accepts relative globs using only * and ?.
// A basename matches at any depth; a pattern containing / starts at the selected
// folder. Negation and ** are deliberately absent so every matching entry is skipped.
func ValidateTransferExclusionPatterns(patterns []string) error {
	if !validTransferExclusionPatterns(patterns) {
		return ErrInvalidTransfer
	}
	return nil
}

func validTransferExclusionPatterns(patterns []string) bool {
	if len(patterns) > MaxTransferExclusionPatterns {
		return false
	}
	for _, pattern := range patterns {
		if pattern == "" || len(pattern) > MaxTransferExclusionPatternBytes || !utf8.ValidString(pattern) ||
			strings.TrimSpace(pattern) != pattern || strings.ContainsAny(pattern, "\\[]!") || strings.Contains(pattern, "**") ||
			strings.IndexFunc(pattern, unicode.IsControl) >= 0 {
			return false
		}
		for _, segment := range strings.Split(pattern, "/") {
			if segment == "" || segment == "." || segment == ".." {
				return false
			}
		}
	}
	return true
}

// TransferExclusionMatcher keeps one invocation's validated rules independent
// of future settings updates. Its zero value excludes nothing.
type TransferExclusionMatcher struct{ patterns transferExclusions }

func NewTransferExclusionMatcher(patterns []string) (TransferExclusionMatcher, error) {
	if err := ValidateTransferExclusionPatterns(patterns); err != nil {
		return TransferExclusionMatcher{}, err
	}
	return TransferExclusionMatcher{patterns: append(transferExclusions(nil), patterns...)}, nil
}

func (matcher TransferExclusionMatcher) Excludes(relativePath string) bool {
	return matcher.patterns.excludes(relativePath)
}

type transferExclusions []string

// excludes includes matching ancestors: no descendant of a skipped directory
// may reappear through a flat browser directory selection.
func (patterns transferExclusions) excludes(relative string) bool {
	for current := relative; current != "" && current != "." && current != "/"; current = path.Dir(current) {
		for _, pattern := range patterns {
			candidate := current
			if !strings.Contains(pattern, "/") {
				candidate = path.Base(current)
			}
			matched, _ := path.Match(pattern, candidate)
			if matched {
				return true
			}
		}
	}
	return false
}

// All folder walkers share the same root-relative rule interpretation.
func (patterns transferExclusions) excludesChild(rootPath, childPath string) bool {
	relative := strings.TrimPrefix(childPath, strings.TrimSuffix(rootPath, "/")+"/")
	return patterns.excludes(relative)
}

func (m *TransferManager) ExcludePatterns() []string {
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	return append([]string{}, m.excludePatterns...)
}

func (m *TransferManager) jobExclusions(id string) ([]string, error) {
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	record := m.jobs[id]
	if record == nil {
		return nil, ErrTransferNotFound
	}
	return append([]string{}, record.job.ExcludePatterns...), nil
}

func (s Service) archiveExclusions() transferExclusions {
	if s.selectedExclusions != nil {
		return transferExclusions(s.selectedExclusions)
	}
	if s.transferExclusions != nil {
		return transferExclusions(s.transferExclusions())
	}
	return nil
}
