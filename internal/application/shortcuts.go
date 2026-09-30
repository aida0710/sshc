package application

import (
	"errors"
	"reflect"
	"regexp"
	"strings"
	"unicode/utf8"

	"sshc/internal/storage"
	"sshc/internal/validate"
)

type ShortcutPreset struct {
	ID       string              `json:"id"`
	Name     string              `json:"name"`
	Bindings map[string][]string `json:"bindings"`
}

var ErrShortcutPresets = errors.New("invalid shortcut presets")
var ErrShortcutConflict = errors.New("shortcut presets changed; reload before saving")
var shortcutID = regexp.MustCompile(`^[a-zA-Z0-9-]{1,64}$`)

// validateShortcutPresets uses the chord grammar, action list and limits of
// internal/validate, which the settings screen receives through rulegen, so a
// chord the screen records is one this accepts.
func validateShortcutPresets(presets []ShortcutPreset) error {
	if len(presets) > validate.MaxShortcutPresets {
		return ErrShortcutPresets
	}
	ids := map[string]bool{}
	for _, p := range presets {
		if !shortcutID.MatchString(p.ID) || p.ID == "default" || ids[p.ID] || strings.TrimSpace(p.Name) == "" || utf8.RuneCountInString(p.Name) > validate.MaxShortcutPresetNameLength || strings.ContainsAny(p.Name, "\r\n\x00") || len(p.Bindings) != len(validate.ShortcutActions) {
			return ErrShortcutPresets
		}
		ids[p.ID] = true
		seen := map[string]bool{}
		for _, action := range validate.ShortcutActions {
			keys, ok := p.Bindings[action]
			if !ok || keys == nil || len(keys) > validate.MaxShortcutKeysPerAction {
				return ErrShortcutPresets
			}
			for _, key := range keys {
				if validate.ShortcutChord(key) != nil || seen[key] {
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
