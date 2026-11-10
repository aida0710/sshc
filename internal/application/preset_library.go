package application

import (
	"reflect"
	"regexp"
	"strings"
	"unicode/utf8"
)

// 名前の付いたプリセットの一覧（ショートカットのプリセット、キーバーのプリセット）は、どれも
// metadata に同期する一覧で、設定のプリセットが ID で1つを選ぶ。ID と名前の決まりと、ほかの端末と
// 取り合ったときの保存の仕方はここにひとつ置く。

// presetLibraryID は、一覧のプリセットの ID の形である。"default" は既定の並びを指すので使えない。
var presetLibraryID = regexp.MustCompile(`^[a-zA-Z0-9-]{1,64}$`)

// validPresetLibraryEntry は、一覧のプリセットの ID と名前が決まりどおりかを返す。
func validPresetLibraryEntry(id, name string, maxNameLength int) bool {
	return presetLibraryID.MatchString(id) && id != "default" &&
		strings.TrimSpace(name) != "" && utf8.RuneCountInString(name) <= maxNameLength && !strings.ContainsAny(name, "\r\n\x00")
}

// replacePresetLibrary は、画面が読んだときの一覧（base）が今も保存されているときだけ、一覧を presets に
// 置き換える。ほかの端末が先に保存していれば conflict を返し、画面に読み直させる。
func replacePresetLibrary[T any](s *Service, operation string, base, presets []T, library func(*Metadata) *[]T, conflict error) (SaveResult, error) {
	return s.changeMetadata(operation, func(stored *Metadata) error {
		current := library(stored)
		// 空の一覧と、一覧が無いことは、同じ初めの状態である。
		if !(len(base) == 0 && len(*current) == 0) && !reflect.DeepEqual(base, *current) {
			return conflict
		}
		*current = presets
		return nil
	})
}

// presetLibraryRules は、一覧の種類（ショートカット、キーバー）ごとの決まりである。
type presetLibraryRules[T any] struct {
	// limit は、一覧に置けるプリセットの数の上限。
	limit int
	id    func(T) string
	// valid は、1つのプリセットの中身が決まりどおりかを返す。
	valid func(T) bool
	// readable は、このビルドが読めない部分を落としたプリセットを返す（readablePresetLibrary）。
	readable func(T) T
}

// validPresetLibrary は、一覧が上限に収まり、どのプリセットも決まりどおりで、ID が重ならないかを返す。
func validPresetLibrary[T any](presets []T, rules presetLibraryRules[T]) bool {
	if len(presets) > rules.limit {
		return false
	}
	ids := map[string]bool{}
	for _, preset := range presets {
		if !rules.valid(preset) || ids[rules.id(preset)] {
			return false
		}
		ids[rules.id(preset)] = true
	}
	return true
}

// readablePresetLibrary は、このビルドが使えるプリセットだけを残す。読めない部分を落としても決まりに合わない
// プリセットは捨てる。残すと、metadata を保存するたびに検証で断られる。1つも残らなければ nil を返す。
func readablePresetLibrary[T any](presets []T, rules presetLibraryRules[T]) []T {
	var kept []T
	ids := map[string]bool{}
	for _, preset := range presets {
		preset = rules.readable(preset)
		if len(kept) >= rules.limit || !rules.valid(preset) || ids[rules.id(preset)] {
			continue
		}
		ids[rules.id(preset)] = true
		kept = append(kept, preset)
	}
	return kept
}
