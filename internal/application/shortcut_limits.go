package application

// ショートカットのプリセットの上限。API（api/openapi.yaml の Metadata.shortcutPresets と
// ShortcutPreset）も同じ値を約束し、internal/acceptance のテストが突き合わせる。
const (
	// MaxShortcutPresets は、保存できるプリセットの数の上限である。
	MaxShortcutPresets = 64
	// MaxShortcutPresetNameRunes は、プリセット名の長さの上限（文字数）である。
	MaxShortcutPresetNameRunes = 80
	// MaxShortcutKeysPerAction は、1つの操作に割り当てられるキーの数の上限である。
	MaxShortcutKeysPerAction = 3
)
