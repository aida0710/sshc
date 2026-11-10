package application

import (
	"errors"

	"sshc/internal/validate"
)

type ShortcutPreset struct {
	ID       string              `json:"id"`
	Name     string              `json:"name"`
	Bindings map[string][]string `json:"bindings"`
}

var ErrShortcutPresets = errors.New("invalid shortcut presets")
var ErrShortcutConflict = errors.New("shortcut presets changed; reload before saving")

// validateShortcutPresets uses the chord grammar, action list and limits of
// internal/validate, which the settings screen receives through rulegen, so a
// chord the screen records is one this accepts.
func validateShortcutPresets(presets []ShortcutPreset) error {
	if !validPresetLibrary(presets, shortcutPresetRules) {
		return ErrShortcutPresets
	}
	return nil
}

// shortcutPresetRules are the rules of the list of shortcut presets. A preset saved before an action was added
// does not bind it: the action is read as unassigned, so the preset the user made stays (the user's choice,
// 2026-11-01). A binding of an action no longer offered is left out.
var shortcutPresetRules = presetLibraryRules[ShortcutPreset]{
	limit: validate.MaxShortcutPresets,
	id:    func(p ShortcutPreset) string { return p.ID },
	valid: validShortcutPreset,
	readable: func(p ShortcutPreset) ShortcutPreset {
		p.Bindings = withEveryShortcutAction(p.Bindings)
		return p
	},
}

// validShortcutPreset says whether p names itself properly and binds every
// action, each to chords the screen records and none twice.
func validShortcutPreset(p ShortcutPreset) bool {
	if !validPresetLibraryEntry(p.ID, p.Name, validate.MaxShortcutPresetNameLength) || len(p.Bindings) != len(validate.ShortcutActions) {
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

// withoutUnreadableShortcutPresets keeps the presets this build can use. A preset still broken after its
// bindings are read (a chord this build does not record, the same chord twice) is dropped.
func withoutUnreadableShortcutPresets(metadata *Metadata) {
	metadata.ShortcutPresets = readablePresetLibrary(metadata.ShortcutPresets, shortcutPresetRules)
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
	return replacePresetLibrary(s, "shortcuts.presets", base, presets, func(stored *Metadata) *[]ShortcutPreset { return &stored.ShortcutPresets }, ErrShortcutConflict)
}
