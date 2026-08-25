package remotesync

import (
	"fmt"
	"sort"
	"strings"
)

// PushDraft is the editable message suggested from the current local diff.
type PushDraft struct {
	Message  string `json:"message"`
	Added    int    `json:"added"`
	Modified int    `json:"modified"`
	Removed  int    `json:"removed"`
}

type manifestChanges struct {
	added    []string
	modified []string
	removed  []string
}

func manifestChanged(base *Manifest, next Manifest) bool {
	changes := diffManifests(base, next)
	return len(changes.added)+len(changes.modified)+len(changes.removed) > 0
}

func diffManifests(base *Manifest, next Manifest) manifestChanges {
	changes := manifestChanges{}
	before := map[string]Entry{}
	if base != nil {
		for _, entry := range base.Files {
			before[entry.Path] = entry
		}
	}
	after := make(map[string]Entry, len(next.Files))
	for _, entry := range next.Files {
		after[entry.Path] = entry
		previous, exists := before[entry.Path]
		switch {
		case !exists:
			changes.added = append(changes.added, entry.Path)
		case previous.SHA256 != entry.SHA256 || previous.Mode != entry.Mode:
			changes.modified = append(changes.modified, entry.Path)
		}
	}
	for _, entry := range before {
		if _, exists := after[entry.Path]; !exists {
			changes.removed = append(changes.removed, entry.Path)
		}
	}
	sort.Strings(changes.added)
	sort.Strings(changes.modified)
	sort.Strings(changes.removed)
	return changes
}

func draftFor(base *Manifest, next Manifest) PushDraft {
	changes := diffManifests(base, next)
	draft := PushDraft{Added: len(changes.added), Modified: len(changes.modified), Removed: len(changes.removed)}
	type namedChange struct{ action, path string }
	items := make([]namedChange, 0, draft.Added+draft.Modified+draft.Removed)
	for _, path := range changes.added {
		items = append(items, namedChange{"Add", path})
	}
	for _, path := range changes.modified {
		items = append(items, namedChange{"Update", path})
	}
	for _, path := range changes.removed {
		items = append(items, namedChange{"Remove", path})
	}
	if len(items) == 0 {
		draft.Message = "Record current workspace"
		return draft
	}
	if len(items) == 1 {
		draft.Message = items[0].action + " " + items[0].path
		return draft
	}
	prefix := fmt.Sprintf("%d changes (+%d ~%d -%d): ", len(items), draft.Added, draft.Modified, draft.Removed)
	paths := make([]string, 0, len(items))
	for _, item := range items {
		paths = append(paths, item.path)
	}
	message := prefix + strings.Join(paths, ", ")
	for len([]rune(message)) > MaxCommitMessageRunes && len(paths) > 1 {
		paths = paths[:len(paths)-1]
		message = fmt.Sprintf("%s%s, and %d more", prefix, strings.Join(paths, ", "), len(items)-len(paths))
	}
	if len([]rune(message)) > MaxCommitMessageRunes {
		runes := []rune(message)
		message = string(runes[:MaxCommitMessageRunes-1]) + "…"
	}
	draft.Message = message
	return draft
}
