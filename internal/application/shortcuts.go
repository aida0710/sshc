package application

import (
	"errors"
	"reflect"
	"regexp"
	"strings"
	"unicode/utf8"

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
		if !validShortcutPreset(p) || ids[p.ID] {
			return ErrShortcutPresets
		}
		ids[p.ID] = true
	}
	return nil
}

// validShortcutPreset says whether p names itself properly and binds every
// action, each to chords the screen records and none twice.
func validShortcutPreset(p ShortcutPreset) bool {
	if !shortcutID.MatchString(p.ID) || p.ID == "default" || strings.TrimSpace(p.Name) == "" || utf8.RuneCountInString(p.Name) > validate.MaxShortcutPresetNameLength || strings.ContainsAny(p.Name, "\r\n\x00") || len(p.Bindings) != len(validate.ShortcutActions) {
		return false
	}
	seen := map[string]bool{}
	for _, action := range validate.ShortcutActions {
		keys, ok := p.Bindings[action]
		if !ok || keys == nil || len(keys) > validate.MaxShortcutKeysPerAction {
			return false
		}
		for _, key := range keys {
			if validate.ShortcutChord(key) != nil || seen[key] {
				return false
			}
			seen[key] = true
		}
	}
	return true
}

// withoutUnreadableShortcutPresets keeps the presets this build can use. A preset saved before an action was
// added does not bind it: the action is read as unassigned, so the preset the user made stays (the user's
// choice, 2026-11-01). A binding of an action no longer offered is left out. A preset still broken after
// that (a chord this build does not record, the same chord twice) is dropped, or every later save of
// metadata would refuse.
func withoutUnreadableShortcutPresets(metadata *Metadata) {
	kept := metadata.ShortcutPresets[:0]
	ids := map[string]bool{}
	for _, p := range metadata.ShortcutPresets {
		p.Bindings = withEveryShortcutAction(p.Bindings)
		if len(kept) >= validate.MaxShortcutPresets || !validShortcutPreset(p) || ids[p.ID] {
			continue
		}
		ids[p.ID] = true
		kept = append(kept, p)
	}
	if len(kept) == 0 {
		kept = nil
	}
	metadata.ShortcutPresets = kept
}

// withEveryShortcutAction is bindings with exactly the actions this build offers: an action missing from it
// is unassigned.
func withEveryShortcutAction(bindings map[string][]string) map[string][]string {
	every := make(map[string][]string, len(validate.ShortcutActions))
	for _, action := range validate.ShortcutActions {
		keys := bindings[action]
		if keys == nil {
			keys = []string{}
		}
		every[action] = keys
	}
	return every
}

func (s *Service) SetShortcutPresets(base, presets []ShortcutPreset) (SaveResult, error) {
	if err := validateShortcutPresets(presets); err != nil {
		return SaveResult{}, err
	}
	return s.changeMetadata("shortcuts.presets", func(stored *Metadata) error {
		// Empty and absent libraries describe the same initial state.
		if !(len(base) == 0 && len(stored.ShortcutPresets) == 0) && !reflect.DeepEqual(base, stored.ShortcutPresets) {
			return ErrShortcutConflict
		}
		stored.ShortcutPresets = presets
		return nil
	})
}
