package application

import (
	"errors"
	"reflect"
	"regexp"
	"strings"
	"unicode/utf8"

	"sshc/internal/storage"
)

type ShortcutPreset struct {
	ID       string              `json:"id"`
	Name     string              `json:"name"`
	Bindings map[string][]string `json:"bindings"`
}

var ErrShortcutPresets = errors.New("invalid shortcut presets")
var ErrShortcutConflict = errors.New("shortcut presets changed; reload before saving")
var shortcutID = regexp.MustCompile(`^[a-zA-Z0-9-]{1,64}$`)
var shortcutChord = regexp.MustCompile("^(Ctrl\\+)?(Alt\\+)?(Shift\\+)?(Meta\\+)?([A-Z0-9]|F([1-9]|1[0-2])|Arrow(Up|Down|Left|Right)|PageUp|PageDown|Home|End|Insert|Delete|Backspace|Enter|Tab|Escape|Space|[-=,.;/\\[\\]\\\\'`])$")
var shortcutFunction = regexp.MustCompile(`^F([1-9]|1[0-2])$`)
var shortcutActions = []string{"palette", "terminalSearch", "copy", "paste", "nextSession", "previousSession", "home", "sftp"}

func validateShortcutPresets(presets []ShortcutPreset) error {
	if len(presets) > 64 {
		return ErrShortcutPresets
	}
	ids := map[string]bool{}
	for _, p := range presets {
		if !shortcutID.MatchString(p.ID) || p.ID == "default" || ids[p.ID] || strings.TrimSpace(p.Name) == "" || utf8.RuneCountInString(p.Name) > 80 || strings.ContainsAny(p.Name, "\r\n\x00") || len(p.Bindings) != len(shortcutActions) {
			return ErrShortcutPresets
		}
		ids[p.ID] = true
		seen := map[string]bool{}
		for _, action := range shortcutActions {
			keys, ok := p.Bindings[action]
			if !ok || keys == nil || len(keys) > 3 {
				return ErrShortcutPresets
			}
			for _, key := range keys {
				if !shortcutChord.MatchString(key) || seen[key] || (!strings.Contains(key, "Ctrl+") && !strings.Contains(key, "Alt+") && !strings.Contains(key, "Meta+") && !shortcutFunction.MatchString(key)) {
					return ErrShortcutPresets
				}
				seen[key] = true
			}
		}
	}
	return nil
}
func (s *Service) SetShortcutPresets(base, presets []ShortcutPreset) (SaveResult, error) {
	if err := validateShortcutPresets(presets); err != nil {
		return SaveResult{}, err
	}
	stored, precondition, err := s.metadata.Load()
	if err != nil {
		return SaveResult{}, err
	}
	// Empty and absent libraries describe the same initial state.
	if !(len(base) == 0 && len(stored.ShortcutPresets) == 0) && !reflect.DeepEqual(base, stored.ShortcutPresets) {
		return SaveResult{}, ErrShortcutConflict
	}
	stored.ShortcutPresets = presets
	if err := s.metadata.EnsureDirectory(); err != nil {
		return SaveResult{}, err
	}
	change, err := s.metadata.Change(stored, precondition)
	if err != nil {
		return SaveResult{}, err
	}
	result, err := s.manager.Commit(storage.Request{Operation: "shortcuts.presets", Changes: []storage.Change{change}})
	if err != nil {
		return SaveResult{}, err
	}
	return SaveResult{TransactionID: result.ID, Written: result.Written}, nil
}
